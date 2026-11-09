package portscaner

import (
	"context"
	"errors"
	"net"
)

type Status string

const (
	StatusOpen        Status = "open"
	StatusClosed      Status = "closed"
	StatusTimeout     Status = "timeout"
	StatusUnreachable Status = "unreachable"
	StatusCanceled    Status = "canceled"
	StatusError       Status = "error"
	StatusParsedError Status = "parsed error"
)

func classifyDialError(err error) Status {
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
