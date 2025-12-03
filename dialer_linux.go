//go:build linux
// +build linux

package main

import (
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
)

// DialBackend effettua la connessione verso l'esterno usando SO_BINDTODEVICE su Linux
func DialBackend(dispatcher *Dispatcher, remoteAddr string) (net.Conn, *Backend, int, error) {
	lb, idx := dispatcher.Next()

	localTCPAddr, err := net.ResolveTCPAddr("tcp4", lb.Address)
	if err != nil {
		return nil, lb, idx, err
	}

	dialer := net.Dialer{
		LocalAddr: localTCPAddr,
		Timeout:   10 * time.Second, // Timeout importante per non bloccare risorse
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// Binding all'interfaccia specifica
				if lb.Interface != "" {
					if err := syscall.BindToDevice(int(fd), lb.Interface); err != nil {
						log.Printf("[WARN] Couldn't bind to interface %s (need sudo?): %v", lb.Interface, err)
					}
				}
			})
		},
	}

	conn, err := dialer.Dial("tcp4", remoteAddr)
	return conn, lb, idx, err
}