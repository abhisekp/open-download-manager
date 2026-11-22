//go:build wasip1

// Original MIT tracker-provider example; networking is supplied by the host.
package main

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"unsafe"
)

//go:wasmimport odm http_get
func httpGet(urlPointer, urlLength, outputPointer, capacity uint32) int32

func main() {
	var input struct {
		API       int    `json:"api"`
		Operation string `json:"operation"`
		URL       string `json:"url"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil || input.API != 2 || input.Operation != "trackers" || input.URL != "https://newtrackon.com/api/live" {
		os.Exit(1)
	}
	endpoint := input.URL
	body := make([]byte, 256<<10)
	count := httpGet(uint32(uintptr(unsafe.Pointer(unsafe.StringData(endpoint)))), uint32(len(endpoint)), uint32(uintptr(unsafe.Pointer(unsafe.SliceData(body)))), uint32(len(body)))
	runtime.KeepAlive(endpoint)
	runtime.KeepAlive(body)
	if count < 0 || int(count) > len(body) {
		os.Exit(1)
	}
	trackers := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(body[:int(count)]), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		if len(trackers) >= 128 {
			os.Exit(1)
		}
		seen[line] = true
		trackers = append(trackers, line)
	}
	if json.NewEncoder(os.Stdout).Encode(map[string]any{"trackers": trackers}) != nil {
		os.Exit(1)
	}
}
