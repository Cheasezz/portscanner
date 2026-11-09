package portscaner

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
)

// ---- Sentinel errors without typed ----
var (
	ErrEmptyHosts       = errors.New("hosts list cant be empty")
	ErrEmptyHost        = errors.New("host cant be empty")
	ErrInvalidPort      = errors.New("invalid port")
	ErrInvalidPortRange = errors.New("invalid port range (min 1, max 65535)")
	ErrWorkersCount     = errors.New("workers count cant be less then 1")
	ErrTimeout          = errors.New("timeout cant be less then 1 sec")
)

// ---- Typed errors ----

var ErrInvalidHost = errors.New("invalid host")

type InvalidHostError struct {
	Host string
}

func (ihs *InvalidHostError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidHost, ihs.Host)
}

func (ihs *InvalidHostError) Unwrap() error {
	return ErrInvalidHost
}

var ErrDupIP = errors.New("duplicated ip")

type DupIPError struct {
	Source string
	IP     netip.Addr
}

func (die *DupIPError) Error() string {
	if die.Source != "" {
		return fmt.Sprintf("%s: %s, source: %s", ErrDupIP, die.IP, die.Source)
	}

	return fmt.Sprintf("%s: %s", ErrDupIP, die.IP)
}

func (die *DupIPError) Unwrap() error {
	return ErrDupIP
}

var ErrLargeSubnet = errors.New("too large subnet")

type LargeSubnetError struct {
	Prefix netip.Prefix
}

func (lse *LargeSubnetError) Error() string {
	return fmt.Sprintf("%s: %s", ErrLargeSubnet, lse.Prefix)
}

func (lse *LargeSubnetError) Unwrap() error {
	return ErrLargeSubnet
}

var ErrDNSTmp = errors.New("cant resolve domain name, temporary error")

type DNSTmpError struct {
	DNSErr *net.DNSError
}

func (de *DNSTmpError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDNSTmp, de.DNSErr.Name)
}

func (de *DNSTmpError) Unwrap() error {
	return ErrDNSTmp
}

var ErrDomainIP = errors.New("domain havent A/AAAA records (also may be domain not exist)")

type DomainIPError struct {
	DNSErr *net.DNSError
}

func (domie *DomainIPError) Error() string {
	if domie.DNSErr == nil {
		return fmt.Sprintf("%s", ErrDomainIP)
	}
	return fmt.Sprintf("%s: %s", ErrDomainIP, domie.DNSErr.Name)
}

func (domie *DomainIPError) Unwrap() error {
	return ErrDomainIP
}

var ErrDNSLookup = errors.New("dns lookup failed")

type DNSLookupError struct {
	DNSErr *net.DNSError
}

func (dle *DNSLookupError) Error() string {
	if dle.DNSErr == nil {
		return fmt.Sprintf("%s", ErrDNSLookup)
	}
	return fmt.Sprintf("%s: %s", ErrDNSLookup, dle.DNSErr.Name)
}

func (dle *DNSLookupError) Unwrap() error {
	return ErrDNSLookup
}

var ErrExcludedPrefix = errors.New("prefix is excluded")

type ExcludedPrefixError struct {
	Prefix netip.Prefix
}

func (epe *ExcludedPrefixError) Error() string {
	return fmt.Sprintf("%s: %s", ErrExcludedPrefix, epe.Prefix)
}

func (epe *ExcludedPrefixError) Unwrap() error {
	return ErrExcludedPrefix
}

var ErrExcludedIp = errors.New("ip in excluded prefix")

type ExcludedIPError struct {
	IP     netip.Addr
	Prefix netip.Prefix
}

func (eie *ExcludedIPError) Error() string {
	return fmt.Sprintf("%s: %s => %s", ErrExcludedIp, eie.IP, eie.Prefix)
}

func (eie *ExcludedIPError) Unwrap() error {
	return ErrExcludedIp
}
