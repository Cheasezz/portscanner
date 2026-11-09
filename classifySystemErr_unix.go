//go:build unix

package portscaner

import (
	"errors"
	"syscall"
)

func classifySystemError(err error) status {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return StatusClosed
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return StatusUnreachable
	default:
		return StatusError
	}
}
