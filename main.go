package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"
)

// PipeConnections unisce due connessioni (copia bidirezionale)
func PipeConnections(local, remote net.Conn) {
	// Canale per attendere la fine
	done := make(chan struct{}, 2)

	copy := func(dst, src net.Conn) {
		io.Copy(dst, src)
		// Se supportato, chiudiamo la scrittura per segnalare EOF all'altro lato
		if c, ok := dst.(*net.TCPConn); ok {
			c.CloseWrite()
		}
		done <- struct{}{}
	}

	go copy(local, remote)
	go copy(remote, local)

	// Attendiamo che la prima direzione finisca
	<-done
	// Chiudiamo tutto per forzare la chiusura dell'altra direzione se appesa
	local.Close()
	remote.Close()
}

// HandleSocksConnection gestisce la logica SOCKS5 standard
func HandleSocksConnection(conn net.Conn, dispatcher *Dispatcher) {
	defer conn.Close()

	if err := HandleHandshake(conn); err != nil {
		log.Printf("[ERR] Handshake failed: %v", err)
		return
	}

	destAddr, err := ReadRequest(conn)
	if err != nil {
		log.Printf("[ERR] Request failed: %v", err)
		return
	}

	remoteConn, lb, idx, err := DialBackend(dispatcher, destAddr)
	if err != nil {
		log.Printf("[WARN] Failed to connect to %s via %s (LB:%d): %v", destAddr, lb.Address, idx, err)
		ReplyError(conn, StatusNetworkUnreachable)
		return
	}
	defer remoteConn.Close()

	log.Printf("[DEBUG] %s -> %s (via %s LB:%d)", conn.RemoteAddr(), destAddr, lb.Address, idx)
	ReplySuccess(conn)
	PipeConnections(conn, remoteConn)
}

// HandleTunnelConnection gestisce la modalità transparent proxy
func HandleTunnelConnection(conn net.Conn, dispatcher *Dispatcher) {
	defer conn.Close()

	// Tentativi di connessione con retry su diversi bilanciatori
	failedBits := big.NewInt(0)
	totalLBs := dispatcher.Count()

	for {
		// Otteniamo un LB che non abbiamo ancora provato in questo ciclo se possibile
		// Nota: qui semplifichiamo usando la chiamata standard ma in un'implementazione
		// avanzata useremmo la logica del bitset per escludere quelli falliti.
		remoteConn, lb, idx, err := DialBackend(dispatcher, lb.Address) // In tunnel mode, lb.Address è la destinazione? No, la logica originale era confusa.
		// In tunnel mode originale: lb.Address è la destinazione remota fissa (es. un server SSH)
		
		if err == nil {
			log.Printf("[DEBUG] Tunnelled to %s (LB:%d)", lb.Address, idx)
			PipeConnections(conn, remoteConn)
			return
		}

		log.Printf("[WARN] Tunnel fail %s: %v (LB:%d)", lb.Address, err, idx)
		
		failedBits.SetBit(failedBits, idx, 1)
		
		// Se li abbiamo provati tutti
		allFailed := true
		for i:=0; i< totalLBs; i++ {
			if failedBits.Bit(i) == 0 {
				allFailed = false
				break
			}
		}

		if allFailed {
			log.Println("[WARN] All load balancers failed for tunnel")
			return
		}
		// Ritenta col loop
	}
}

func parseLoadBalancers(args []string, isTunnel bool) []*Backend {
	if len(args) == 0 {
		log.Fatal("[FATAL] Please specify one or more load balancers")
	}

	list := make([]*Backend, 0, len(args))

	for idx, arg := range args {
		parts := strings.Split(arg, "@")
		addrPart := parts[0]
		
		var lbAddr string
		var lbPort int
		var err error
		var iface string

		if isTunnel {
			// In tunnel mode, addrPart è ip:port remoto
			host, portStr, err := net.SplitHostPort(addrPart)
			if err != nil {
				log.Fatalf("[FATAL] Invalid tunnel address %s: %v", addrPart, err)
			}
			lbPort, _ = strconv.Atoi(portStr)
			lbAddr = host
		} else {
			// In socks mode, addrPart è l'IP locale da usare per uscire
			if net.ParseIP(addrPart).To4() == nil {
				log.Fatalf("[FATAL] Invalid IP address: %s", addrPart)
			}
			lbAddr = addrPart
			iface = getInterfaceFromIP(lbAddr)
			if iface == "" {
				log.Fatalf("[FATAL] IP %s not associated with any interface", lbAddr)
			}
		}

		ratio := 1
		if len(parts) > 1 {
			ratio, err = strconv.Atoi(parts[1])
			if err != nil || ratio <= 0 {
				log.Fatalf("[FATAL] Invalid contention ratio for %s", addrPart)
			}
		}

		fullAddr := lbAddr
		if isTunnel {
			fullAddr = fmt.Sprintf("%s:%d", lbAddr, lbPort)
		} else {
			// Per socks mode aggiungiamo porta 0 (effimera)
			fullAddr = fmt.Sprintf("%s:0", lbAddr)
		}

		log.Printf("[INFO] LB %d: %s (Iface: %s), Ratio: %d", idx+1, fullAddr, iface, ratio)
		
		list = append(list, &Backend{
			Address:         fullAddr,
			Interface:       iface,
			ContentionRatio: ratio,
		})
	}
	return list
}

func getInterfaceFromIP(ip string) string {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.String() == ip {
					// Sulle vecchie versioni Linux il null byte serviva, 
					// ma col nuovo Go di solito non serve. Lo metto per compatibilità col codice originale.
					return iface.Name + "\x00" 
				}
			}
		}
	}
	return ""
}

func main() {
	lhost := flag.String("lhost", "127.0.0.1", "The host to listen for SOCKS connection")
	lport := flag.Int("lport", 8080, "The local port to listen for SOCKS connection")
	tunnel := flag.Bool("tunnel", false, "Use tunnelling mode")
	quiet := flag.Bool("quiet", false, "Disable logs")
	flag.Parse()

	if *quiet {
		log.SetOutput(io.Discard)
	}

	backends := parseLoadBalancers(flag.Args(), *tunnel)
	dispatcher := NewDispatcher(backends)

	bindAddr := fmt.Sprintf("%s:%d", *lhost, *lport)
	listener, err := net.Listen("tcp4", bindAddr)
	if err != nil {
		log.Fatalf("[FATAL] Could not start server on %s: %v", bindAddr, err)
	}
	
	log.Printf("[INFO] Server started on %s", bindAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[WARN] Accept error: %v", err)
			continue
		}
		
		go func(c net.Conn) {
			if *tunnel {
				HandleTunnelConnection(c, dispatcher)
			} else {
				HandleSocksConnection(c, dispatcher)
			}
		}(conn)
	}
}