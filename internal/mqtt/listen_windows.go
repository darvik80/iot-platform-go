//go:build windows

package mqtt

import "syscall"

// Windows SO_EXCLUSIVEADDRUSE = ~SO_REUSEADDR (0x0004). Not always exported
// from syscall / x/sys in current Go.
const soExclusiveAddrUse = ^0x4

// exclusiveControl prevents a second listener on the same port.
// Go's default SO_REUSEADDR on Windows lets RabbitMQ/WinNAT and the platform
// both show LISTENING on :1883; ESP then hits the wrong broker (EOF on CONNECT).
func exclusiveControl(_ string, _ string, c syscall.RawConn) error {
	var sockErr error
	if err := c.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
	}); err != nil {
		return err
	}
	return sockErr
}
