package portscaner

import (
	"context"
	"net"
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
	Target target
	// Status is classify dial err. If no err - StatusOpen
	Status Status
	// Store dial err
	Err error
}

// dial return empty res with only Target field if target has parsing error
func dial(ctx context.Context, t target, timeout time.Duration) Result {
	res := Result{Target: t}

	if t.Err != nil {
		return res
	}

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, t.network, t.String())
	if err != nil {
		res.Status = classifyDialError(err)
		res.Err = err
		return res
	}
	conn.Close()

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
