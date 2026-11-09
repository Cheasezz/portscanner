package portscaner

import (
	"fmt"
	"strconv"
	"strings"
)

// Input can be mixed row port and range ports.
// Range must be like "1-10" (from min to max, between char "-")
// If input is empty or nil - method use default ports range.
func parsePorts(s []string) (map[int]struct{}, error) {
	if len(s) == 0 {
		s = []string{defaultPorts}
	}

	ports := make(map[int]struct{})

	for _, part := range s {
		part = strings.TrimSpace(part)

		// Expand range ports
		if strings.Contains(part, "-") {
			err := expandPorts(ports, part)
			if err != nil {
				return nil, err
			}
		} else {
			p, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidPort, part)
			}

			if p < 1 || p > 65535 {
				return nil, fmt.Errorf("%w: %d", ErrInvalidPortRange, p)
			}

			if _, ok := ports[p]; !ok {
				ports[p] = struct{}{}
			}
		}
	}

	return ports, nil
}

func expandPorts(ports map[int]struct{}, part string) error {
	bounds := strings.SplitN(part, "-", 2)

	start, err := strconv.Atoi(bounds[0])
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidPort, part)
	}

	end, err := strconv.Atoi(bounds[1])
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidPort, part)
	}

	if start < 1 || end > 65535 || start > end {
		return fmt.Errorf("%w: %s", ErrInvalidPortRange, part)
	}

	for p := start; p <= end; p++ {
		if _, ok := ports[p]; !ok {
			ports[p] = struct{}{}
		}
	}

	return nil
}
