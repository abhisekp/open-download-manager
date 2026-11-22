//go:build wasip1

package main

import (
	"encoding/json"
	"os"
)

func main() {
	var input struct {
		Operation string `json:"operation"`
		URL       string `json:"url"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil || input.Operation != "trackers" || input.URL != "https://example.com/feed" {
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"trackers": []string{"udp://tracker.example.net:6969/announce", "https://tracker.example.org/announce"}})
}
