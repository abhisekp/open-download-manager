//go:build wasip1

// Original abuse-test fixture for the API 2 memory/request boundaries.
package main

import (
	"encoding/json"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"unsafe"
)

//go:wasmimport odm http_get
func httpGet(urlPointer, urlLength, outputPointer, capacity uint32) int32

func main() {
	var input struct {
		URL string `json:"url"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		os.Exit(1)
	}
	source, err := url.Parse(input.URL)
	if err != nil {
		os.Exit(1)
	}
	target := source.Query().Get("target")
	if target == "" {
		target = "https://httpbin.org/fixture"
	}
	output := make([]byte, 256<<10)
	inputPointer := uint32(uintptr(unsafe.Pointer(unsafe.StringData(target))))
	inputLength := uint32(len(target))
	outputPointer := uint32(uintptr(unsafe.Pointer(unsafe.SliceData(output))))
	capacity := uint32(len(output))
	switch source.Query().Get("mode") {
	case "bad-input":
		inputPointer = ^uint32(0)
	case "bad-output":
		outputPointer = ^uint32(0)
	case "large-input":
		inputLength = 8193
	case "large-output":
		capacity = 256<<10 + 1
	case "empty-output":
		capacity = 0
	}
	attempts := 1
	if value, err := strconv.Atoi(source.Query().Get("attempts")); err == nil && value >= 1 && value <= 5 {
		attempts = value
	}
	results := make([]int32, 0, attempts)
	for range attempts {
		results = append(results, httpGet(inputPointer, inputLength, outputPointer, capacity))
	}
	runtime.KeepAlive(target)
	runtime.KeepAlive(output)
	if source.Query().Get("mode") == "resolve" && len(results) == 1 && results[0] == -1 {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"urls": []string{input.URL}})
		return
	}
	if json.NewEncoder(os.Stdout).Encode(map[string]any{"results": results}) != nil {
		os.Exit(1)
	}
}
