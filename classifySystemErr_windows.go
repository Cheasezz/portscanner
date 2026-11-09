//go:build windows

package portscanner

import (
	"errors"
	"syscall"
)

const (
	wsaConnRefused syscall.Errno = 10061
	wsaHostUnreach syscall.Errno = 10065
	wsaNetUnreach  syscall.Errno = 10051
)

func classifySystemError(err error) Status {
	switch {
	case errors.Is(err, wsaConnRefused):
		return StatusClosed
	case errors.Is(err, wsaHostUnreach), errors.Is(err, wsaNetUnreach):
		return StatusUnreachable
	default:
		return StatusError
	}
}
