// Test-only sample proving that the host grants neither files, environment nor sockets.
package main

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"time"
)

func main() {
	var input struct {
		URL string `json:"url"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		os.Exit(2)
	}
	u, err := url.Parse(input.URL)
	if err != nil {
		os.Exit(2)
	}
	query := url.Values{}
	if _, err = os.ReadFile(u.Query().Get("file")); err != nil {
		query.Set("files", "denied")
	} else {
		query.Set("files", "accessible")
	}
	if os.Getenv("ODM_SANDBOX_PROBE") == "" {
		query.Set("environment", "empty")
	} else {
		query.Set("environment", "leaked")
	}
	connection, err := net.DialTimeout("tcp", u.Query().Get("socket"), 50*time.Millisecond)
	if err != nil {
		query.Set("network", "denied")
	} else {
		query.Set("network", "accessible")
		_ = connection.Close()
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"urls": []string{"https://example.com/probe?" + query.Encode()}})
}
