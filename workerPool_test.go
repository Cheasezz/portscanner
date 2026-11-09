package portscanner

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTimeout = 5 * time.Second

func makeTargets(count int) <-chan target {
	targets := make(chan target, count)
	ip := netip.MustParseAddr("127.0.0.1")

	for i := range count {
		targets <- target{
			source:  ip.String(),
			ip:      ip,
			port:    i + 1,
			network: defaultNetwork,
		}
	}

	close(targets)
	return targets
}

func makeTestPool(
	t *testing.T,
	workers int,
	targets <-chan target,
	dialFn func(context.Context, target, time.Duration) Result,
) *pool {
	t.Helper()

	p, err := newPool(workers, targets, time.Second)
	require.NoError(t, err)

	p.dial = dialFn

	return p
}

func waitSignals(t *testing.T, ch <-chan struct{}, count int) {
	t.Helper()

	timer := time.NewTimer(testTimeout)
	defer timer.Stop()

	for range count {
		select {
		case <-ch:
		case <-timer.C:
			t.Fatal("timeout waiting for signals")
		}
	}
}

func collectResults(t *testing.T, results <-chan Result) []Result {
	t.Helper()

	collected := make([]Result, 0)

	timer := time.NewTimer(testTimeout)
	defer timer.Stop()

	for {
		select {
		case res, ok := <-results:
			if !ok {
				return collected
			}

			collected = append(collected, res)

		case <-timer.C:
			t.Fatal("timeout waiting for results")
			return nil
		}
	}
}

func updateMax(max *atomic.Int32, value int32) {
	for {
		old := max.Load()

		if value <= old || max.CompareAndSwap(old, value) {
			return
		}
	}
}

func TestPool_ConcurrencyLimit(t *testing.T) {
	const (
		workers = 4
		total   = 20
	)

	var (
		active    atomic.Int32
		maxActive atomic.Int32
		calls     atomic.Int32
	)

	started := make(chan struct{}, total)
	release := make(chan struct{})

	var releaseOnce sync.Once
	releaseAll := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	t.Cleanup(releaseAll)

	fakeDial := func(
		ctx context.Context,
		tgt target,
		timeout time.Duration,
	) Result {
		current := active.Add(1)
		defer active.Add(-1)

		calls.Add(1)
		updateMax(&maxActive, current)

		started <- struct{}{}
		<-release

		return Result{
			Source: tgt.source,
			IP:     tgt.ip,
			Port:   tgt.port,
			Status: StatusOpen,
		}
	}

	p := makeTestPool(t, workers, makeTargets(total), fakeDial)

	p.startPool(context.Background())

	waitSignals(t, started, workers)

	assert.Equal(t, int32(workers), active.Load())
	assert.Equal(t, int32(workers), maxActive.Load())

	select {
	case <-started:
		t.Fatal("more workers started than configured")
	default:
	}

	releaseAll()

	results := collectResults(t, p.results)

	assert.Len(t, results, total)
	assert.Equal(t, int32(total), calls.Load())
	assert.Equal(t, int32(workers), maxActive.Load())
	assert.Zero(t, active.Load())
}

func TestPool_Cancel(t *testing.T) {
	const (
		workers = 4
		total   = 20
	)

	var calls atomic.Int32

	started := make(chan struct{}, total)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fakeDial := func(
		ctx context.Context,
		tgt target,
		timeout time.Duration,
	) Result {
		calls.Add(1)
		started <- struct{}{}

		<-ctx.Done()

		return Result{
			Source: tgt.source,
			IP:     tgt.ip,
			Port:   tgt.port,
			Status: StatusCanceled,
			Err:    ctx.Err(),
		}
	}

	p := makeTestPool(t, workers, makeTargets(total), fakeDial)

	p.startPool(ctx)

	waitSignals(t, started, workers)

	cancel()

	results := collectResults(t, p.results)

	require.Len(t, results, workers)

	for _, result := range results {
		assert.Equal(t, StatusCanceled, result.Status)
		assert.ErrorIs(t, result.Err, context.Canceled)
	}

	assert.Equal(t, int32(workers), calls.Load())
}

func TestPool_ResultsClosed(t *testing.T) {
	const workers = 2

	started := make(chan struct{}, workers)
	release := make(chan struct{}, workers)
	fakeDial := func(
		ctx context.Context,
		tgt target,
		timeout time.Duration,
	) Result {
		started <- struct{}{}
		<-release

		return Result{
			Source: tgt.source,
			IP:     tgt.ip,
			Port:   tgt.port,
			Status: StatusOpen,
		}
	}

	p := makeTestPool(t, workers, makeTargets(workers), fakeDial)

	p.startPool(context.Background())

	waitSignals(t, started, workers)

	release <- struct{}{}

	timer := time.NewTimer(testTimeout)
	defer timer.Stop()

	select {
	case _, ok := <-p.results:
		require.True(t, ok, "results channel closed before all workers finished")
	case <-timer.C:
		t.Fatal("timeout waiting for first result")
	}

	select {
	case _, ok := <-p.results:
		assert.True(t, ok, "results channel closed before all workers finished")
	default:
	}

	release <- struct{}{}

	select {
	case _, ok := <-p.results:
		require.True(t, ok, "expected second result")
	case <-timer.C:
		t.Fatal("timeout waiting for second result")
	}

	select {
	case _, ok := <-p.results:
		require.False(t, ok, "expected results channel to be closed")
	case <-timer.C:
		t.Fatal("results channel was not closed")
	}
}

func TestPool_SlowConsumer(t *testing.T) {
	const (
		workers = 4
		total   = 600
	)

	var calls atomic.Int32

	fakeDial := func(
		ctx context.Context,
		tgt target,
		timeout time.Duration,
	) Result {
		calls.Add(1)

		return Result{
			Source: tgt.source,
			IP:     tgt.ip,
			Port:   tgt.port,
			Status: StatusOpen,
		}
	}

	p := makeTestPool(t, workers, makeTargets(total), fakeDial)

	p.startPool(context.Background())

	timer := time.NewTimer(testTimeout)
	defer timer.Stop()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for len(p.results) < cap(p.results) {
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatalf(
				"results buffer was not filled: len=%d, cap=%d",
				len(p.results),
				cap(p.results),
			)
		}
	}

	assert.Less(t, calls.Load(), int32(total))

	results := collectResults(t, p.results)

	assert.Len(t, results, total)
	assert.Equal(t, int32(total), calls.Load())
}

func TestScanner_ConcurrentScans(t *testing.T) {
	const (
		scansCount = 5
		workers    = 10
	)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	portStr := strconv.Itoa(port)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			_ = conn.Close()
		}
	}()

	scanner, err := New(
		WithWorkers(workers),
		WithTimeout(time.Second),
	)
	require.NoError(t, err)

	type scanResult struct {
		id      int
		results []Result
		err     error
	}

	start := make(chan struct{})
	results := make(chan scanResult, scansCount)

	var wg sync.WaitGroup

	for i := range scansCount {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			<-start

			ctx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()

			resultChan, err := scanner.Scan(
				ctx,
				[]string{"127.0.0.1"},
				[]string{portStr},
			)
			if err != nil {
				results <- scanResult{id: id, err: err}
				return
			}

			var collected []Result

			for result := range resultChan {
				collected = append(collected, result)
			}

			results <- scanResult{
				id:      id,
				results: collected,
			}
		}(i)
	}

	close(start)

	wg.Wait()
	close(results)

	for scan := range results {
		require.NoError(t, scan.err, "scan %d returned an error", scan.id)

		require.Len(t, scan.results, 1,
			"scan %d returned unexpected result count", scan.id)

		assert.Equal(t, "127.0.0.1", scan.results[0].Source)
		assert.Equal(t, port, scan.results[0].Port)
		assert.Equal(t, StatusOpen, scan.results[0].Status)
	}
}
