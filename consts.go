package main

// SOCKS5 Protocol Constants
const (
	SocksVersion5 = 0x05
	ReservedField = 0x00
)

// Authentication Methods
const (
	AuthNoAuth             = 0x00
	AuthGSSAPI             = 0x01
	AuthUsernamePassword   = 0x02
	AuthNoAcceptableMethod = 0xFF
)

// Commands
const (
	CmdConnect      = 0x01
	CmdBind         = 0x02
	CmdUDPAssociate = 0x03
)

// Address Types
const (
	AddrTypeIPv4   = 0x01
	AddrTypeDomain = 0x03
	AddrTypeIPv6   = 0x04
)

// Request Status (Replies)
const (
	StatusSuccess              = 0x00
	StatusServerFailure        = 0x01
	StatusConnectionNotAllowed = 0x02
	StatusNetworkUnreachable   = 0x03
	StatusHostUnreachable      = 0x04
	StatusConnectionRefused    = 0x05
	StatusTTLExpired           = 0x06
	StatusCommandNotSupported  = 0x07
	StatusAddrTypeNotSupported = 0x08
)