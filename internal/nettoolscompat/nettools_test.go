package nettools

import (
	"testing"
	"time"
)

func TestCompactAddresses(t *testing.T) {
	for _, a := range []string{"127.0.0.1:6881", "[2001:db8::1]:65535"} {
		if got := BinaryToDottedPort(DottedPortToBinary(a)); got != a {
			t.Fatalf("%s became %s", a, got)
		}
	}
	for _, a := range []string{"example.com:80", "127.0.0.1:65536", "invalid"} {
		if DottedPortToBinary(a) != "" {
			t.Fatal("accepted", a)
		}
	}
	if BinaryToDottedPort("short") != "" {
		t.Fatal("accepted malformed compact address")
	}
}
func TestThrottleBoundaries(t *testing.T) {
	c := NewThrottler(2, 2)
	if c.CheckBlock("a") || c.CheckBlock("a") || !c.CheckBlock("a") {
		t.Fatal("wrong threshold")
	}
	c.entries["a"] = window{start: time.Now().Add(-time.Minute)}
	if c.CheckBlock("a") {
		t.Fatal("window did not reset")
	}
	c.CheckBlock("b")
	c.CheckBlock("c")
	if len(c.entries) > 2 {
		t.Fatal("unbounded hosts")
	}
	c.Stop()
	if !c.CheckBlock("x") {
		t.Fatal("stopped limiter accepted a request")
	}
}
