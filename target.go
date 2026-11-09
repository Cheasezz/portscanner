package portscaner

import (
	"context"
	"net"
	"net/netip"
	"strconv"
)

const (
	defaultNetwork = "tcp"
	defaultPorts   = "1-1000"
)

type target struct {
	// The Source field stores the value indicating
	// the origin of the ip address (domain name, cidr, single ip)
	Source string
	IP     netip.Addr
	Port   int
	// Only tcp now
	network string
	// Store host parsing err. All errors in errors.go
	Err error
}

// Join host:port for target
func (t *target) String() string {
	return net.JoinHostPort(t.IP.String(), strconv.Itoa(t.Port))
}

func genereateTargets(ctx context.Context, hosts, ports []string) (<-chan target, error) {
	targets := make(chan target, 500)

	parsedPorts, err := parsePorts(ports)
	if err != nil {
		return nil, err
	}

	hostsSeq, err := parseHosts(ctx, hosts)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(targets)

		for host, hostErr := range hostsSeq {
			if ctx.Err() != nil {
				return
			}
			for port := range parsedPorts {
				select {
				case <-ctx.Done():
					return
				case targets <- target{
					Source:  host.source,
					IP:      host.ip,
					Port:    port,
					network: defaultNetwork,
					Err:     hostErr,
				}:
				}
			}
		}
	}()

	return targets, nil
}
