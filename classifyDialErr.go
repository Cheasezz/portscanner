package portscaner

import (
	"context"
	"errors"
	"net"
)

type status string

const (
	StatusOpen        status = "open"
	StatusClosed      status = "closed"
	StatusTimeout     status = "timeout"
	StatusUnreachable status = "unreachable"
	StatusCanceled    status = "canceled"
	StatusError       status = "error"
)

func classifyDialError(err error) status {
	if errors.Is(err, context.Canceled) {
		return StatusCanceled
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return StatusTimeout
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return StatusTimeout
	}

	return classifySystemError(err)
}
