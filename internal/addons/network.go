package addons

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const providerResponseLimit = 256 << 10
const providerTotalLimit = 512 << 10

var providerExcluded = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("168.63.129.16/32"), // cloud platform service, not a provider endpoint
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}
var providerGlobalIPv6 = netip.MustParsePrefix("2000::/3")

func providerPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || address.Zone() != "" || !address.IsGlobalUnicast() || address.IsPrivate() {
		return false
	}
	if address.Is6() && !providerGlobalIPv6.Contains(address) {
		return false
	}
	for _, prefix := range providerExcluded {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// Transport hooks are private, supplied only by same-package tests. Signed
// manifests and worker requests cannot alter DNS, dialing or TLS verification.
type providerTransportHooks struct {
	lookup func(context.Context, string) ([]net.IPAddr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
	tls    *tls.Config
}

func providerClient(hooks *providerTransportHooks) *http.Client {
	lookup := net.DefaultResolver.LookupIPAddr
	dial := (&net.Dialer{Timeout: 2 * time.Second}).DialContext
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if hooks != nil {
		if hooks.lookup != nil {
			lookup = hooks.lookup
		}
		if hooks.dial != nil {
			dial = hooks.dial
		}
		if hooks.tls != nil {
			tlsConfig = hooks.tls.Clone()
		}
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: tlsConfig, ForceAttemptHTTP2: true, MaxIdleConns: 2, MaxIdleConnsPerHost: 1, IdleConnTimeout: 5 * time.Second, TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 3 * time.Second, MaxResponseHeaderBytes: 16 << 10}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" {
			return nil, errors.New("provider port refused")
		}
		dnsContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		addresses, err := lookup(dnsContext, host)
		if err != nil || len(addresses) == 0 || len(addresses) > 16 {
			return nil, errors.New("provider DNS unavailable")
		}
		checked := make([]netip.Addr, 0, len(addresses))
		for _, resolved := range addresses {
			ip, ok := netip.AddrFromSlice(resolved.IP)
			if !ok || resolved.Zone != "" || !providerPublicAddress(ip) {
				return nil, errors.New("provider DNS includes a refused address")
			}
			checked = append(checked, ip.Unmap())
		}
		for _, ip := range checked {
			connection, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return connection, nil
			}
			if ctx.Err() != nil {
				break
			}
		}
		return nil, errors.New("provider connection unavailable")
	}
	return &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirects refused") }}
}

func providerURL(raw string, hosts []string) bool {
	if len(raw) == 0 || len(raw) > 8192 || strings.IndexFunc(raw, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Hostname() == "" || (parsed.Port() != "" && parsed.Port() != "443") {
		return false
	}
	for _, host := range hosts {
		if strings.EqualFold(host, parsed.Hostname()) {
			return true
		}
	}
	return false
}

type providerBudget struct{ attempts, received int }

func (b *providerBudget) get(ctx context.Context, client *http.Client, hosts []string, raw string, capacity uint32) ([]byte, error) {
	b.attempts++
	if b.attempts > 4 || capacity == 0 || capacity > providerResponseLimit || b.received >= providerTotalLimit || !providerURL(raw, hosts) {
		return nil, errors.New("provider request refused")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, errors.New("invalid provider request")
	}
	request.Header.Set("User-Agent", "OpenDownloadManager-Provider/2")
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("provider request unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("provider returned an unsupported response")
	}
	limit := min(int(capacity), providerTotalLimit-b.received)
	if response.ContentLength > int64(limit) {
		return nil, errors.New("provider response too large")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	b.received += len(body)
	if err != nil || len(body) > limit {
		return nil, errors.New("provider response too large or interrupted")
	}
	return body, nil
}
