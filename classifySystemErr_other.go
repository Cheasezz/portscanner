//go:build !unix && !windows

package portscaner

func classifySystemError(err error) Status {
	return StatusError
}
