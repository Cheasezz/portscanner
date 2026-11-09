package portscanner

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

type pool struct {
	workers        int
	results        chan Result
	targets        <-chan target
	timeoutPerDial time.Duration
	// Need for change dial in tests
	dial func(ctx context.Context, t target, timeout time.Duration) Result
}

type Result struct {
	// The Source field stores the value indicating
	// the origin of the ip address (domain name, cidr, single ip)
	Source string
	IP     netip.Addr
	Port   int
	// Status can be classified dial err/ parsed status err/ ok
	Status   Status
	Duration time.Duration
	Err      error
}

func (r *Result) String() string {
	addr := net.JoinHostPort(r.IP.String(), strconv.Itoa(r.Port))
	return fmt.Sprintf("Source: %s, addr: %s, status: %s, time: %s", r.Source, addr, r.Status, r.Duration)
}

func dial(ctx context.Context, t target, timeout time.Duration) Result {
	res := Result{}

	if t.err != nil {
		res.Status = StatusParsedError
		res.Err = t.err
		return res
	}

	res.Source = t.source
	res.IP = t.ip
	res.Port = t.port

	start := time.Now()

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, t.network, t.String())
	if err != nil {
		res.Status = classifyDialError(err)
		res.Duration = time.Since(start)
		res.Err = err
		return res
	}
	conn.Close()

	res.Duration = time.Since(start)
	res.Status = StatusOpen
	return res
}

func newPool(workers int, targets <-chan target, timeoutPerDial time.Duration) (*pool, error) {
	p := &pool{
		workers:        workers,
		results:        make(chan Result, 500),
		targets:        targets,
		timeoutPerDial: timeoutPerDial,
		dial:           dial,
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
			// If use select for ctx.Done, then worker may process
			// several targets after  ctx cancelation, because
			// select cant guarantee use ctx cancel branch if both cases is ready.
			// Check ctx like this can guarantee that after cancelation
			// one worker do dial maximum one more time and send result to chan.
			if !ok || ctx.Err() != nil {
				return
			}

			p.results <- p.dial(ctx, t, p.timeoutPerDial)
		}
	}
}
