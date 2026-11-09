
# Port Scanner

A concurrent TCP Connect port scanner written in Go.

The package provides an embeddable API for scanning TCP ports on IPv4 addresses, IPv6 addresses, domain names, and CIDR ranges. It uses a configurable worker pool, supports context cancellation, and returns structured results through a channel.

## Features

- TCP Connect scanning.
- IPv4 and IPv6 support.
- Domain name resolution (A and AAAA records).
- CIDR range expansion with limits to prevent excessively large scans.
- Port numbers and port ranges.
- Configurable number of concurrent workers.
- Configurable connection timeout.
- Context cancellation.
- Structured scan results with connection status and duration.
- Platform-specific classification of some network errors.

## Requirements

- Go 1.25.5 or later.

## Installation

```bash
go get github.com/Cheasezz/portscanner
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Cheasezz/portscanner"
)

func main() {
	scanner, err := portscanner.New(
		portscanner.WithWorkers(100),
	)
	if err != nil {
		log.Fatal(err)
	}

	hosts := []string{
		"127.0.0.1",
		"example.com",
		"192.168.1.0/28",
	}

	ports := []string{
		"80",
		"443",
		"8000-8010",
	}

	results, err := scanner.Scan(context.Background(), hosts, ports)
	if err != nil {
		log.Fatal(err)
	}

	for result := range results {
		fmt.Printf(
			"%s:%d — %s (%s)\n",
			result.IP,
			result.Port,
			result.Status,
			result.Duration,
		)

		if result.Err != nil {
			fmt.Printf("  error: %v\n", result.Err)
		}
	}
}
```

`Scan` returns a receive-only channel. Results are delivered asynchronously, so the caller should read from the channel until it is closed.

## Public API

### Creating a scanner

```go
func New(opt ...option) (*Scanner, error)
```

Creates a new scanner instance.

The scanner accepts functional options:

| Option | Description | Default |
|---|---|---|
| `WithWorkers(count int)` | Maximum number of concurrent workers per scan | `500` |
| `WithTimeout(timeout time.Duration)` | Timeout for an individual TCP connection attempt | `2 seconds` |

Example:

```go
scanner, err := portscanner.New(
	portscanner.WithWorkers(200),
	portscanner.WithTimeout(3*time.Second),
)
if err != nil {
	return err
}
```

The constructor returns an error if the worker count is less than 1 or the timeout is not positive.

Scanner configuration is immutable after construction. The same scanner instance can be used for multiple concurrent scans.

### Starting a scan

```go
func (s *Scanner) Scan(
	ctx context.Context,
	hosts, ports []string,
) (<-chan Result, error)
```

Starts a scan and returns a channel of results.

**Arguments:**

- `ctx` — controls the lifetime of the scan. Use it to cancel ongoing work.
- `hosts` — a list of IP addresses, domain names, or CIDR ranges.
- `ports` — a list of individual ports or port ranges.

The method returns an error if the input cannot be initialized, for example, when the host list is empty or the port specification is invalid.

Host resolution and validation errors are reported as scan results with `StatusParsedError`, rather than being returned directly by `Scan`.

### Host formats

The `hosts` argument accepts a mixture of the following formats:

```go
hosts := []string{
	"192.168.1.10",      // IPv4
	"2001:db8::1",       // IPv6
	"example.com",       // Domain name
	"192.168.1.0/28",    // IPv4 CIDR
	"2001:db8::/112",    // IPv6 CIDR
}
```

Domain names are resolved using the system DNS resolver. All returned IPv4 and IPv6 addresses are considered scan targets.

CIDR ranges are expanded into individual IP addresses. To prevent excessive expansion, the following limits apply:

- IPv4 prefixes must be `/16` or smaller in address count (prefix length `/16` or greater).
- IPv6 prefixes must have a prefix length of at least `/112`.

For example, `192.168.1.0/24` is accepted, while `10.0.0.0/8` is rejected.

Some technically reserved address ranges are excluded, including unspecified, multicast, and reserved IPv4 ranges, as well as unspecified and multicast IPv6 addresses.

Duplicate IP addresses are detected across all supplied hosts, including addresses obtained through DNS resolution and CIDR expansion. Duplicate entries are reported as parsing errors.

### Port formats

The `ports` argument accepts individual ports and inclusive ranges:

```go
ports := []string{
	"80",
	"443",
	"8000-8010",
	"8080",
}
```

Individual ports and ranges can be mixed. Duplicate port numbers are automatically deduplicated.

Valid port numbers are in the range `1-65535`.

If `ports` is `nil` or empty, the scanner uses the default range `1-1000`.

Invalid port numbers, malformed ranges, and ranges with a start value greater than the end value produce an error.

### Scan results

Each scan result is represented by the `Result` structure:

```go
type Result struct {
	Source   string
	IP       netip.Addr
	Port     int
	Status   Status
	Duration time.Duration
	Err      error
}
```

| Field | Description |
|---|---|
| `Source` | Original host specification that produced the IP address (IP, domain, or CIDR) |
| `IP` | IP address being scanned |
| `Port` | TCP port being scanned |
| `Status` | Result of the connection attempt or host parsing |
| `Duration` | Time spent on the connection attempt |
| `Err` | Underlying error, if any |

For parsing errors, some target-specific fields may be zero-valued because no valid IP/port target could be constructed.

### Scan statuses

The `Status` type represents the outcome of a scan:

| Status | Meaning |
|---|---|
| `StatusOpen` | TCP connection was successfully established |
| `StatusClosed` | Connection was explicitly refused |
| `StatusTimeout` | Connection attempt timed out |
| `StatusUnreachable` | Destination or network is unreachable |
| `StatusCanceled` | Connection attempt was canceled through the context |
| `StatusError` | Other connection error that could not be classified |
| `StatusParsedError` | Host parsing, validation, or DNS resolution error |

Example:

```go
switch result.Status {
case portscanner.StatusOpen:
	// Port is accepting TCP connections.
case portscanner.StatusClosed:
	// Connection was refused.
case portscanner.StatusTimeout:
	// Connection attempt timed out.
case portscanner.StatusUnreachable:
	// Destination or network is unreachable.
case portscanner.StatusCanceled:
	// Connection attempt was canceled.
case portscanner.StatusParsedError:
	// Host could not be parsed or resolved.
case portscanner.StatusError:
	// Other connection error.
}
```

The scanner uses Go's error inspection mechanisms (`errors.Is` and `errors.As`) rather than matching error strings.

Some network errors are classified using operating-system-specific error codes. Errors that cannot be classified on the current platform are reported as `StatusError`.

## Context cancellation

The scanner uses `context.Context` to control scan execution.

When the context is canceled:

- No new targets are scheduled for processing after workers observe cancellation.
- Already-started connection attempts receive the canceled context.
- Results from workers that have already started processing are still sent to the results channel.
- The results channel is closed after all workers finish.

Example:

```go
ctx, cancel := context.WithCancel(context.Background())

results, err := scanner.Scan(ctx, hosts, ports)
if err != nil {
	return err
}

// Cancel the scan when needed.
cancel()

// Continue reading until the channel is closed.
for result := range results {
	// Handle results.
}
```

**Important:** Continue consuming the results channel until it is closed, even after cancellation. If the consumer stops reading, workers may block while sending results.

## Error handling

The package exposes sentinel errors and typed errors that can be inspected using the standard `errors` package.

For example:

```go
if errors.Is(err, portscanner.ErrEmptyHosts) {
	// The host list is empty.
}
```

For typed errors:

```go
var invalidHostErr *portscanner.InvalidHostError

if errors.As(result.Err, &invalidHostErr) {
	fmt.Println("Invalid host:", invalidHostErr.Host)
}
```

### Sentinel errors

| Error | Description |
|---|---|
| `ErrEmptyHosts` | The host list is empty |
| `ErrEmptyHost` | An empty host entry was provided |
| `ErrInvalidPort` | A port value could not be parsed |
| `ErrInvalidPortRange` | A port is outside the valid range or a range is invalid |
| `ErrWorkersCount` | Worker count is less than 1 |
| `ErrTimeout` | Timeout is not positive |
| `ErrInvalidHost` | Host format is invalid |
| `ErrDupIP` | Duplicate IP address |
| `ErrLargeSubnet` | CIDR range exceeds the allowed expansion limit |
| `ErrDNSTmp` | Temporary DNS resolution error |
| `ErrDomainIP` | Domain has no A/AAAA records or could not be found |
| `ErrDNSLookup` | Other DNS lookup error |
| `ErrExcludedPrefix` | CIDR prefix is excluded |
| `ErrExcludedIp` | IP address belongs to an excluded prefix |

### Typed errors

The package also exposes typed errors with additional context:

- `InvalidHostError` — contains the invalid host in `Host`.
- `DupIPError` — contains the duplicate IP in `IP` and its source in `Source`.
- `LargeSubnetError` — contains the rejected prefix in `Prefix`.
- `DNSTmpError` — contains the underlying DNS error in `DNSErr`.
- `DomainIPError` — contains the DNS error in `DNSErr`.
- `DNSLookupError` — contains the DNS error in `DNSErr`.
- `ExcludedPrefixError` — contains the excluded prefix in `Prefix`.
- `ExcludedIPError` — contains the excluded IP in `IP` and matching prefix in `Prefix`.

Typed errors can be inspected with `errors.As`. Their corresponding sentinel errors can be checked with `errors.Is`.

## Concurrency

Each call to `Scan` creates an independent worker pool.

The number of workers is configured per scanner instance and limits the number of concurrent connection attempts within a single scan.

Multiple scans can be executed concurrently using the same scanner instance. Each scan has its own targets and results channels.

Results are delivered through a buffered channel. If the consumer reads results slower than workers produce them, workers may block while sending results. This provides backpressure instead of allowing the result queue to grow without bound.

## Limitations

- Only TCP Connect scanning is supported.
- The package does not implement raw packet scanning or SYN scanning.
- The scanner does not determine whether a service is running on an open port; it only checks whether a TCP connection can be established.
- Results depend on network reachability, firewall rules, DNS configuration, and operating-system behavior.
- Large CIDR ranges are intentionally restricted.

