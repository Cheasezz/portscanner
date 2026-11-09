package portscaner

import (
	"context"
	"time"
)

var (
	defaultWorkersCount     = 500
	defaultTimeoutPerTarget = time.Second * 2
)

type option func(s *Scaner)

type Scaner struct {
	// Workers count per scan. Default 500
	workersPerScan int
	// Timeout per network call. Default 2 sec
	timout time.Duration
}

// New accept [option] args. Available options:
//   - [WithWorkers] - setup workers count per scan.
//   - [WithTimeout] - setup timeout per network call.
func New(opt ...option) (*Scaner, error) {
	s := &Scaner{
		workersPerScan: defaultWorkersCount,
		timout:         defaultTimeoutPerTarget,
	}

	for _, opt := range opt {
		opt(s)
	}

	if s.workersPerScan < 1 {
		return nil, ErrWorkersCount
	}

	return s, nil
}

// Scan run goroutine, need ctx for cancelation.
//   - Hosts can be mixed format etc. {"127.0.0.1","domain.com","192.168.0.55/24"}.
//   - CIDR cant be less then /16 and /112 for ip4 and ip6.
//   - Ports can be mixed format etc. {"80","443","1000-1500"}.
//   - Ports can be empty or nil, in this case automaticaly added default ports (1-1000).
func (s *Scaner) Scan(ctx context.Context, hosts, ports []string) (<-chan Result, error) {
	targets, err := genereateTargets(ctx, hosts, ports)
	if err != nil {
		return nil, err
	}

	pool, err := newPool(s.workersPerScan, targets, s.timout)
	if err != nil {
		return nil, err
	}

	resChan := pool.startPool(ctx)

	return resChan, nil
}

func WithWorkers(count int) option {
	return func(s *Scaner) {
		s.workersPerScan = count
	}
}

func WithTimeout(timeout time.Duration) option {
	return func(s *Scaner) {
		s.timout = timeout
	}
}
