package linkpreview

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

const (
	maxURLBytes = 2048
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// ParseURL validates and canonicalizes an HTTP(S) target without resolving DNS.
func ParseURL(raw string) (*url.URL, error) {
	if len(raw) > maxURLBytes {
		return nil, errors.New("invalid_url")
	}
	target, err := url.Parse(raw)
	if err != nil || target.Opaque != "" || target.User != nil || target.Host == "" {
		return nil, errors.New("invalid_url")
	}
	target.Scheme = strings.ToLower(target.Scheme)
	if target.Scheme != schemeHTTP && target.Scheme != schemeHTTPS {
		return nil, errors.New("invalid_url")
	}
	port := target.Port()
	if strings.HasSuffix(target.Host, ":") ||
		(port != "" && ((target.Scheme == schemeHTTP && port != "80") || (target.Scheme == schemeHTTPS && port != "443"))) {
		return nil, errors.New("invalid_url")
	}
	hostname := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	if addr, parseErr := netip.ParseAddr(hostname); parseErr == nil {
		if !publicAddress(addr) {
			return nil, errors.New("blocked_address")
		}
		hostname = addr.String()
		target.Host = hostname
		if addr.Is6() {
			target.Host = "[" + hostname + "]"
		}
	} else {
		if strings.Contains(hostname, ":") || strings.Contains(hostname, "%") {
			return nil, errors.New("invalid_url")
		}
		hostname, err = idna.Lookup.ToASCII(hostname)
		if err != nil || !validHostname(hostname) {
			return nil, errors.New("invalid_url")
		}
		target.Host = hostname
	}
	target.Fragment = ""
	target.RawFragment = ""
	return target, nil
}

func validHostname(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") || host == "localhost" {
		return false
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".home.arpa"} {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return false
		}
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return false
			}
		}
	}
	return true
}

func publicAddress(addr netip.Addr) bool {
	if !addr.IsValid() || addr.Zone() != "" {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	if addr.Is4() {
		for _, block := range []string{
			"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
			"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.31.196.0/24", "192.52.193.0/24",
			"192.88.99.0/24", "192.168.0.0/16", "192.175.48.0/24", "198.18.0.0/15", "198.51.100.0/24",
			"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		} {
			if netip.MustParsePrefix(block).Contains(addr) {
				return false
			}
		}
		return true
	}
	return publicIPv6Address(addr)
}

func publicIPv6Address(addr netip.Addr) bool {
	for _, block := range []string{"2001::/23", "2001:db8::/32", "2002::/16", "2620:4f:8000::/48", "3fff::/20"} {
		if netip.MustParsePrefix(block).Contains(addr) {
			return false
		}
	}
	// IANA reserves unlisted global-unicast ranges; 2000::/3 alone also admits retired 6bone.
	// https://www.iana.org/assignments/ipv6-unicast-address-assignments/ (2025-10-10 registry)
	for _, block := range []string{
		"2001:200::/23", "2001:400::/23", "2001:600::/23", "2001:800::/22",
		"2001:c00::/23", "2001:e00::/23", "2001:1200::/23", "2001:1400::/22",
		"2001:1800::/23", "2001:1a00::/23", "2001:1c00::/22", "2001:2000::/19",
		"2001:4000::/23", "2001:4200::/23", "2001:4400::/23", "2001:4600::/23",
		"2001:4800::/23", "2001:4a00::/23", "2001:4c00::/23", "2001:5000::/20",
		"2001:8000::/19", "2001:a000::/20", "2001:b000::/20", "2003::/18",
		"2400::/12", "2410::/12", "2600::/12", "2610::/23", "2620::/23",
		"2630::/12", "2800::/12", "2a00::/12", "2a10::/12", "2c00::/12",
	} {
		if netip.MustParsePrefix(block).Contains(addr) {
			return true
		}
	}
	return false
}

func destination(target *url.URL, addr netip.Addr) string {
	port := "80"
	if target.Scheme == schemeHTTPS {
		port = "443"
	}
	return net.JoinHostPort(addr.String(), port)
}
