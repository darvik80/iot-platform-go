//go:build !windows

package mqtt

import "syscall"

func exclusiveControl(_ string, _ string, _ syscall.RawConn) error {
	return nil
}
