package portscaner

import (
	"context"
	"errors"
	"iter"
	"net"
	"net/netip"
	"slices"
	"strings"
)

var excludedPrefixes = []netip.Prefix{
	// ip4 unspecified
	netip.MustParsePrefix("0.0.0.0/8"),
	// ip4 multicast
	netip.MustParsePrefix("224.0.0.0/4"),
	// reserved ip4 range
	netip.MustParsePrefix("240.0.0.0/4"),
	// ip6 unspecified
	netip.MustParsePrefix("::/128"),
	// ip6 multicast
	netip.MustParsePrefix("ff00::/8"),
}

type parsedHost struct {
	source string
	ip     netip.Addr
}

type (
	hostYield  func(parsedHost, error) bool
	dupHostMap map[netip.Addr]struct{}
)

func (dhm dupHostMap) Add(ip netip.Addr) bool {
	if dhm == nil {
		return false
	}

	ip = ip.Unmap()

	if _, ok := dhm[ip]; ok {
		return false
	}

	dhm[ip] = struct{}{}
	return true
}

// The input may contain a mix of ip, domains and CIDRs.
// Expand string s into iterator[parsedHost, error].
func parseHosts(ctx context.Context, s []string) (iter.Seq2[parsedHost, error], error) {
	if len(s) == 0 {
		return nil, ErrEmptyHosts
	}

	// This prevents mutation if user change host slice after start scan.
	s = slices.Clone(s)

	return func(yield func(parsedHost, error) bool) {
		// Map for checking duplicated hosts.
		dupHosts := make(dupHostMap)

		for _, part := range s {

			part = strings.TrimSpace(part)
			if len(part) == 0 {
				if !yield(parsedHost{}, ErrEmptyHost) {
					return
				}
				continue
			}

			if parsed, next := tryParseLikeIP(part, dupHosts, yield); parsed {
				if !next {
					return
				}
				continue
			}

			if parsed, next := tryParseLikeCIDR(part, dupHosts, yield); parsed {
				if !next {
					return
				}
				continue
			}

			if parsed, next := tryParseLikeDomain(ctx, part, dupHosts, yield); parsed {
				if !next {
					return
				}
				continue
			}

			if !yield(parsedHost{}, &InvalidHostError{Host: part}) {
				return
			}

		}
	}, nil
}

func tryParseLikeIP(part string, dupHosts dupHostMap, yield hostYield) (parsed, next bool) {
	parsed, next = false, true

	ip, err := netip.ParseAddr(part)
	if err != nil {
		return parsed, next
	}
	parsed = true

	if err := validateIP(ip); err != nil {
		if !yield(parsedHost{}, err) {
			next = false
		}
		return parsed, next
	}

	if !dupHosts.Add(ip) {
		if !yield(parsedHost{}, &DupIPError{Source: part, IP: ip}) {
			next = false
		}
		return parsed, next
	}

	next = yield(parsedHost{source: part, ip: ip}, nil)

	return parsed, next
}

func tryParseLikeCIDR(part string, dupHosts dupHostMap, yield hostYield) (parsed, next bool) {
	parsed, next = false, true

	prefix, err := netip.ParsePrefix(part)
	if err != nil {
		return parsed, next
	}

	parsed = true

	prefix = prefix.Masked()

	// Prevent expansion of excluded prefixes.
	if slices.Contains(excludedPrefixes, prefix) {
		next = yield(parsedHost{}, &ExcludedPrefixError{Prefix: prefix})
		return parsed, next
	}

	if !isSafePrefixSize(prefix) {
		next = yield(parsedHost{}, &LargeSubnetError{Prefix: prefix})
		return parsed, next
	}

	// Expand subnet
	for ip := prefix.Addr(); prefix.Contains(ip); ip = ip.Next() {
		if err := validateIP(ip); err != nil {
			if !yield(parsedHost{}, err) {
				next = false
				return parsed, next
			}
			continue
		}

		if !dupHosts.Add(ip) {
			if !yield(parsedHost{}, &DupIPError{Source: part, IP: ip}) {
				next = false
				return parsed, next
			}
			continue
		}

		if !yield(parsedHost{source: part, ip: ip}, nil) {
			next = false
			return parsed, next
		}
	}

	return parsed, next
}

func tryParseLikeDomain(ctx context.Context, part string, dupHosts dupHostMap, yield hostYield) (parsed, next bool) {
	parsed, next = true, true

	// May be use golang.org/x/net/idna for parsing domain before lookup
	// but dependencies of this package require go version >= 1.26.0
	domainIps, err := net.DefaultResolver.LookupNetIP(ctx, "ip", part)
	if err != nil {
		//	Not context.Cause() because the main task is to handle the err,
		//	not find out reason of context cancelation.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			next = false
			return parsed, next
		}

		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {

			if dnsErr.Timeout() || dnsErr.Temporary() {
				next = yield(parsedHost{}, &DNSTmpError{DNSErr: dnsErr})
				return parsed, next
			}

			// Domain has no A/AAAA records, also may be domain not exist.
			// As this lib is for port scanning,
			// i dont want to check whether the domain name itself exists.
			if dnsErr.IsNotFound {
				next = yield(parsedHost{}, &DomainIPError{DNSErr: dnsErr})
				return parsed, next
			}

			// Some else dnsErr
			next = yield(parsedHost{}, &DNSLookupError{DNSErr: dnsErr})
			return parsed, next
		}
		// Something else lookup err
		parsed = false
		return parsed, next
	}

	for _, ip := range domainIps {
		if err := validateIP(ip); err != nil {
			if !yield(parsedHost{}, err) {
				next = false
				return parsed, next
			}
			continue
		}

		if !dupHosts.Add(ip) {
			if !yield(parsedHost{}, &DupIPError{Source: part, IP: ip}) {
				next = false
				return parsed, next
			}
			continue
		}

		if !yield(parsedHost{source: part, ip: ip}, nil) {
			next = false
			return parsed, next
		}
	}

	return parsed, next
}

// Prevent expansion of large subnets.
func isSafePrefixSize(prefix netip.Prefix) bool {
	netBits := prefix.Bits()

	if prefix.Addr().Is4() {
		return netBits >= 16
	}

	if prefix.Addr().Is6() {
		return netBits >= 112
	}

	return false
}

// Validate meens check if [excludedPrefixes] contains ip.
func validateIP(ip netip.Addr) error {
	if !ip.IsValid() {
		return &InvalidHostError{Host: ip.String()}
	}
	unmapIP := ip.Unmap()

	for _, pref := range excludedPrefixes {
		if pref.Contains(unmapIP) {
			return &ExcludedIPError{IP: ip, Prefix: pref}
		}
	}

	return nil
}
