package portscaner

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"
)

type pool struct {
	workers        int
	results        chan Result
	targets        <-chan target
	timeoutPerDial time.Duration
}

type Result struct {
	Source   string
	IP       netip.Addr
	Port     int
	Status   Status
	Duration time.Duration
	Err      error
}

func dial(ctx context.Context, t target, timeout time.Duration) Result {
	res := Result{}

	if t.Err != nil {
		res.Status = StatusParsedError
		res.Err = t.Err
		return res
	}

	res.Source = t.Source
	res.IP = t.IP
	res.Port = t.Port

	start := time.Now()

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, t.network, t.String())
	if err != nil {
		res.Status = classifyDialError(err)
		res.Err = err
		return res
	}
	conn.Close()
	dur := time.Since(start)

	res.Duration = dur
	res.Status = StatusOpen
	return res
}

func newPool(workers int, targets <-chan target, timeoutPerDial time.Duration) (*pool, error) {
	p := &pool{
		workers:        workers,
		results:        make(chan Result, 500),
		targets:        targets,
		timeoutPerDial: timeoutPerDial,
	}

	return p, nil
}

func (p *pool) startPool(ctx context.Context) <-chan Result {
	var wg sync.WaitGroup

	for range p.workers {
		wg.Go(func() {
			p.spawn(ctx)
		})
	}

	go func() {
		wg.Wait()
		close(p.results)
	}()

	return p.results
}

func (p *pool) spawn(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case t, ok := <-p.targets:
			if !ok {
				return
			}

			res := dial(ctx, t, p.timeoutPerDial)

			select {
			case <-ctx.Done():
				return
			case p.results <- res:
			}
		}
	}
}
