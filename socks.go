package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

// HandleHandshake gestisce la negoziazione iniziale (versione e auth)
func HandleHandshake(conn net.Conn) error {
	// Leggiamo header: Versione + Num Methods
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("handshake read error: %v", err)
	}

	if buf[0] != SocksVersion5 {
		return errors.New("unsupported SOCKS version")
	}

	numMethods := int(buf[1])
	methods := make([]byte, numMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return fmt.Errorf("reading auth methods error: %v", err)
	}

	// Per ora supportiamo solo NO AUTH. 
	// In futuro qui si potrebbe ciclare su 'methods' per cercare auth specifici.
	
	// Rispondiamo che accettiamo NO AUTH (0x00)
	if _, err := conn.Write([]byte{SocksVersion5, AuthNoAuth}); err != nil {
		return fmt.Errorf("handshake write error: %v", err)
	}

	return nil
}

// ReadRequest legge la richiesta di connessione (CMD, ADDR, PORT)
func ReadRequest(conn net.Conn) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", fmt.Errorf("request header read error: %v", err)
	}

	ver, cmd, _, addrType := header[0], header[1], header[2], header[3]

	if ver != SocksVersion5 {
		return "", errors.New("unsupported SOCKS version")
	}

	if cmd != CmdConnect {
		ReplyError(conn, StatusCommandNotSupported)
		return "", fmt.Errorf("unsupported command: %d", cmd)
	}

	var destAddr string
	var portBytes = make([]byte, 2)

	switch addrType {
	case AddrTypeIPv4:
		ipv4 := make([]byte, 4)
		if _, err := io.ReadFull(conn, ipv4); err != nil {
			return "", err
		}
		if _, err := io.ReadFull(conn, portBytes); err != nil {
			return "", err
		}
		destAddr = fmt.Sprintf("%s:%d", net.IP(ipv4).String(), binary.BigEndian.Uint16(portBytes))

	case AddrTypeDomain:
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenByte); err != nil {
			return "", err
		}
		domain := make([]byte, int(lenByte[0]))
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		if _, err := io.ReadFull(conn, portBytes); err != nil {
			return "", err
		}
		destAddr = fmt.Sprintf("%s:%d", string(domain), binary.BigEndian.Uint16(portBytes))

	case AddrTypeIPv6:
		ipv6 := make([]byte, 16)
		if _, err := io.ReadFull(conn, ipv6); err != nil {
			return "", err
		}
		if _, err := io.ReadFull(conn, portBytes); err != nil {
			return "", err
		}
		destAddr = fmt.Sprintf("[%s]:%d", net.IP(ipv6).String(), binary.BigEndian.Uint16(portBytes))

	default:
		ReplyError(conn, StatusAddrTypeNotSupported)
		return "", errors.New("address type not supported")
	}

	return destAddr, nil
}

// ReplyError invia un messaggio di errore al client
func ReplyError(conn net.Conn, status byte) {
	// Risposta standard di errore: Ver 5, Status, Rsv 0, Type IPv4, 0.0.0.0, Port 0
	conn.Write([]byte{SocksVersion5, status, 0x00, AddrTypeIPv4, 0, 0, 0, 0, 0, 0})
}

// ReplySuccess invia conferma di connessione avvenuta
func ReplySuccess(conn net.Conn) {
	// Rispondiamo con successo bindando su 0.0.0.0:0 (dummy)
	conn.Write([]byte{SocksVersion5, StatusSuccess, 0x00, AddrTypeIPv4, 0, 0, 0, 0, 0, 0})
}