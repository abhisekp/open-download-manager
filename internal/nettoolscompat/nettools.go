// SPDX-License-Identifier: MIT
// Original ODM compatibility code for compact peer addresses and bounded throttling.
// This implements only the API surface required by the upstream DHT dependency.
package nettools

import (
	"encoding/binary"
	"net"
	"strconv"
	"sync"
	"time"
)

func BinaryToDottedPort(compact string) string {
	b := []byte(compact)
	if len(b) != 6 && len(b) != 18 {
		return ""
	}
	ip := net.IP(b[:len(b)-2])
	port := binary.BigEndian.Uint16(b[len(b)-2:])
	return net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))
}
func DottedPortToBinary(address string) string {
	host, portText, e := net.SplitHostPort(address)
	if e != nil {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	port, e := strconv.ParseUint(portText, 10, 16)
	if e != nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	} else {
		ip = ip.To16()
	}
	out := make([]byte, len(ip)+2)
	copy(out, ip)
	binary.BigEndian.PutUint16(out[len(ip):], uint16(port))
	return string(out)
}

type window struct {
	start time.Time
	hits  int
}
type ClientThrottle struct {
	mu              sync.Mutex
	limit, capacity int
	entries         map[string]window
	stopped         bool
}

func NewThrottler(maxPerMinute int, maxHosts int64) *ClientThrottle {
	if maxHosts < 1 {
		maxHosts = 1
	}
	if maxHosts > 100000 {
		maxHosts = 100000
	}
	return &ClientThrottle{limit: maxPerMinute, capacity: int(maxHosts), entries: map[string]window{}}
}
func (c *ClientThrottle) CheckBlock(host string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return true
	}
	now := time.Now()
	w, exists := c.entries[host]
	if !exists && len(c.entries) >= c.capacity {
		var oldest string
		var earliest time.Time
		for k, v := range c.entries {
			if oldest == "" || v.start.Before(earliest) {
				oldest = k
				earliest = v.start
			}
		}
		delete(c.entries, oldest)
	}
	if !exists || now.Sub(w.start) >= time.Minute {
		w = window{start: now}
	}
	w.hits++
	c.entries[host] = w
	return c.limit <= 0 || w.hits > c.limit
}
func (c *ClientThrottle) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	c.entries = map[string]window{}
}
