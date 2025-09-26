package kcp2k

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

// 全局缓冲池用于GenerateCookie函数
var cookieBufferPool = New(func() []byte {
	return make([]byte, 4)
})

// ResolveHostname resolves hostname to IP addresses
// Returns the resolved addresses and a boolean indicating success
func ResolveHostname(hostname string) ([]net.IP, bool) {
	// NOTE: dns lookup is blocking. this can take a second.
	addresses, err := net.LookupIP(hostname)
	if err != nil {
		Log.Error(fmt.Sprintf("[KCP] Common: Failed to resolve host: %s reason: %v", hostname, err))
		return nil, false
	}
	return addresses, len(addresses) >= 1
}

// ConfigureSocketBuffers configures socket buffer sizes
// If connections drop under heavy load, increase to OS limit.
// If still not enough, increase the OS limit.
func ConfigureSocketBuffers(conn *net.UDPConn, recvBufferSize, sendBufferSize int) error {
	err := conn.SetDeadline(time.Time{})
	if err != nil {
		return err
	}

	err = conn.SetReadBuffer(recvBufferSize)
	if err != nil {
		return err
	}
	err = conn.SetWriteBuffer(sendBufferSize)
	if err != nil {
		return err
	}
	return nil
}

// ConnectionHash generates a connection hash from IP+Port
// NOTE: We use a simple hash combination of IP and Port
// Different connections should have different hashes
func ConnectionHash(addr net.Addr) int {
	if addr == nil {
		return 0
	}

	switch a := addr.(type) {
	case *net.UDPAddr:
		// Combine IP bytes and port for hash
		hash := 0
		if a.IP != nil {
			for _, b := range a.IP {
				hash = hash*31 + int(b)
			}
		}
		hash = hash*31 + a.Port
		return hash
	case *net.TCPAddr:
		// Combine IP bytes and port for hash
		hash := 0
		if a.IP != nil {
			for _, b := range a.IP {
				hash = hash*31 + int(b)
			}
		}
		hash = hash*31 + a.Port
		return hash
	default:
		// Fallback to string hash
		s := addr.String()
		hash := 0
		for _, c := range s {
			hash = hash*31 + int(c)
		}
		return hash
	}
}

// GenerateCookie generates a secure random cookie
// Cookies need to be generated with a secure random generator.
// We don't want them to be deterministic / predictable.
func GenerateCookie() uint32 {
	buf := cookieBufferPool.Get()
	defer func() {
		// 重置缓冲区并归还到池中
		// 使用clear()函数更高效地清零缓冲区
		clear(buf)
		cookieBufferPool.Put(buf)
	}()

	_, err := rand.Read(buf)
	if err != nil {
		// Fallback to a simple method if crypto/rand fails
		// This should rarely happen
		return uint32(len(buf)) // Simple fallback
	}
	return binary.LittleEndian.Uint32(buf)
}
