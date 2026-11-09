//go:build !unix && !windows

package portscanner

func classifySystemError(err error) Status {
	return StatusError
}
