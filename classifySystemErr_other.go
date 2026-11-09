//go:build !unix && !windows

package portscaner

func classifySystemError(err error) status {
	return StatusError
}
