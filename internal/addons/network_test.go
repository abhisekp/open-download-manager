package addons

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func providerFixture(t *testing.T, handler http.HandlerFunc) (*http.Client, *providerTransportHooks, string) {
	return providerFixtureForHost(t, handler, "provider.example.test")
}

func providerFixtureForHost(t *testing.T, handler http.HandlerFunc, host string) (*http.Client, *providerTransportHooks, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	hooks := &providerTransportHooks{tls: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, lookup: func(ctx context.Context, query string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}, dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "93.184.216.34:443" {
			return nil, errors.New("unvalidated destination dialed")
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	client := providerClient(hooks)
	t.Cleanup(client.CloseIdleConnections)
	return client, hooks, "https://" + host
}

func TestProviderAddressAndURLBoundaries(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "0.1.2.3", "10.1.2.3", "100.100.100.200", "169.254.169.254", "172.16.1.1", "192.168.1.1", "192.0.0.192", "192.0.2.1", "198.18.0.1", "198.51.100.2", "203.0.113.2", "224.0.0.1", "255.255.255.255", "168.63.129.16", "::", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "2001:db8::1", "2002:7f00:1::", "64:ff9b::7f00:1", "3fff::1"} {
		if providerPublicAddress(netip.MustParseAddr(raw)) {
			t.Errorf("reserved/private address accepted: %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		if !providerPublicAddress(netip.MustParseAddr(raw)) {
			t.Errorf("public address refused: %s", raw)
		}
	}
	hosts := []string{"provider.example.test"}
	for _, raw := range []string{"http://provider.example.test", "https://provider.example.test:444/", "https://user:secret@provider.example.test/", "https://provider.example.test/#fragment", "https://other.example.test/", "https://provider.example.test/a b", "file:///private"} {
		if providerURL(raw, hosts) {
			t.Fatal("unapproved provider URL accepted")
		}
	}
	if !providerURL("https://provider.example.test:443/metadata?fixture=1", hosts) {
		t.Fatal("approved provider URL refused")
	}
}

func TestProviderPinnedTLSNoProxyCredentialsOrRedirects(t *testing.T) {
	var calls atomic.Int32
	client, _, endpoint := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" {
			t.Error("ambient credentials or unsupported method sent")
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "https://127.0.0.1/private", 302)
			return
		}
		_, _ = w.Write([]byte("original provider response"))
	})
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyCalls.Add(1); w.WriteHeader(502) }))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	budget := &providerBudget{}
	result, err := budget.get(context.Background(), client, []string{"provider.example.test"}, endpoint+"/data", 1024)
	if err != nil || string(result) != "original provider response" || proxyCalls.Load() != 0 {
		t.Fatal("pinned provider GET or proxy isolation failed", err)
	}
	if _, err = budget.get(context.Background(), client, []string{"provider.example.test"}, endpoint+"/redirect", 1024); err == nil {
		t.Fatal("redirect followed")
	}
	if calls.Load() != 2 {
		t.Fatal("redirect caused extra requests")
	}
}

func TestProviderDNSPrivateMixedAndRebindingRefused(t *testing.T) {
	for _, ips := range [][]string{{"127.0.0.1"}, {"93.184.216.34", "10.0.0.1"}, {"::ffff:169.254.169.254"}} {
		var dials atomic.Int32
		hooks := &providerTransportHooks{lookup: func(context.Context, string) ([]net.IPAddr, error) {
			var result []net.IPAddr
			for _, ip := range ips {
				result = append(result, net.IPAddr{IP: net.ParseIP(ip)})
			}
			return result, nil
		}, dial: func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, errors.New("must not dial")
		}}
		client := providerClient(hooks)
		_, err := (&providerBudget{}).get(context.Background(), client, []string{"provider.example.test"}, "https://provider.example.test/", 1024)
		client.CloseIdleConnections()
		if err == nil || dials.Load() != 0 {
			t.Fatal("private/mixed DNS reached a connection")
		}
	}
	client, hooks, endpoint := providerFixture(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("original")) })
	client.CloseIdleConnections()
	var lookups atomic.Int32
	hooks.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		if lookups.Add(1) > 1 {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	pinned := providerClient(hooks)
	defer pinned.CloseIdleConnections()
	budget := &providerBudget{}
	if _, err := budget.get(context.Background(), pinned, []string{"provider.example.test"}, endpoint, 1024); err != nil {
		t.Fatal(err)
	}
	pinned.CloseIdleConnections()
	if _, err := budget.get(context.Background(), pinned, []string{"provider.example.test"}, endpoint, 1024); err == nil {
		t.Fatal("DNS rebinding accepted")
	}
}

func TestProviderResponseAttemptAndTLSLimits(t *testing.T) {
	var calls atomic.Int32
	client, hooks, endpoint := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/large" {
			_, _ = w.Write([]byte(strings.Repeat("x", providerResponseLimit+1)))
		} else {
			_, _ = w.Write([]byte("original"))
		}
	})
	budget := &providerBudget{}
	if _, err := budget.get(context.Background(), client, []string{"provider.example.test"}, endpoint+"/large", providerResponseLimit); err == nil {
		t.Fatal("oversized provider body accepted")
	}
	budget = &providerBudget{}
	for range 4 {
		if _, err := budget.get(context.Background(), client, []string{"provider.example.test"}, endpoint, 1024); err != nil {
			t.Fatal(err)
		}
	}
	before := calls.Load()
	if _, err := budget.get(context.Background(), client, []string{"provider.example.test"}, endpoint, 1024); err == nil || calls.Load() != before {
		t.Fatal("attempt budget exceeded")
	}
	wrong := providerClient(hooks)
	defer wrong.CloseIdleConnections()
	if _, err := (&providerBudget{}).get(context.Background(), wrong, []string{"wrong.example.test"}, "https://wrong.example.test/", 1024); err == nil {
		t.Fatal("TLS hostname verification bypassed")
	}
	budget = &providerBudget{received: providerTotalLimit}
	before = calls.Load()
	if _, err := budget.get(context.Background(), client, []string{"provider.example.test"}, endpoint, 1024); err == nil || calls.Load() != before {
		t.Fatal("total body budget bypassed")
	}
}

func TestProviderCancellationClosesPendingRequest(t *testing.T) {
	entered := make(chan struct{})
	client, _, endpoint := providerFixture(t, func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (&providerBudget{}).get(ctx, client, []string{"provider.example.test"}, endpoint, 1024)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("provider fixture never received request")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled provider request succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("provider cancellation did not finish")
	}
}
