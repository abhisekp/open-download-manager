//go:build wasip1

// This API 2 example sends the reviewed input link to httpbin.org's public echo
// service. Use synthetic links only; it is an SDK demonstration, not a provider.
package main

import (
	"encoding/json"
	"net/url"
	"os"
	"runtime"
	"unsafe"
)

//go:wasmimport odm http_get
func httpGet(urlPointer, urlLength, outputPointer, capacity uint32) int32

func main() {
	var input struct {
		API int    `json:"api"`
		URL string `json:"url"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil || input.API != 2 {
		os.Exit(1)
	}
	endpoint := "https://httpbin.org/anything?url=" + url.QueryEscape(input.URL)
	output := make([]byte, 256<<10)
	count := httpGet(uint32(uintptr(unsafe.Pointer(unsafe.StringData(endpoint)))), uint32(len(endpoint)), uint32(uintptr(unsafe.Pointer(unsafe.SliceData(output)))), uint32(len(output)))
	runtime.KeepAlive(endpoint)
	runtime.KeepAlive(output)
	if count < 0 || int(count) > len(output) {
		os.Exit(1)
	}
	var response struct {
		Args struct {
			URL string `json:"url"`
		} `json:"args"`
	}
	if json.Unmarshal(output[:int(count)], &response) != nil || response.Args.URL == "" {
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(map[string]any{"urls": []string{response.Args.URL}}) != nil {
		os.Exit(1)
	}
}
