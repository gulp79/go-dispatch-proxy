//go:build !linux
// +build !linux

package main

import (
	"net"
	"time"
)

// DialBackend per sistemi non-Linux (nessun BindToDevice)
func DialBackend(dispatcher *Dispatcher, remoteAddr string) (net.Conn, *Backend, int, error) {
	lb, idx := dispatcher.Next()

	localTCPAddr, _ := net.ResolveTCPAddr("tcp4", lb.Address)
	remoteTCPAddr, err := net.ResolveTCPAddr("tcp4", remoteAddr)
	if err != nil {
		return nil, lb, idx, err
	}

	// Usiamo net.Dialer per avere il timeout
	dialer := net.Dialer{
		LocalAddr: localTCPAddr,
		Timeout:   10 * time.Second,
	}

	conn, err := dialer.Dial("tcp4", remoteTCPAddr.String())
	return conn, lb, idx, err
}