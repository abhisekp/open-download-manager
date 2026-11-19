// Build with GOOS=wasip1 GOARCH=wasm. The module gets no network or file access.
package main

import (
	"encoding/json"
	"net/url"
	"os"
)

func main() {
	var input struct {
		API       int    `json:"api"`
		Operation string `json:"operation"`
		URL       string `json:"url"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil || input.API != 1 || input.Operation != "resolve" {
		os.Exit(2)
	}
	u, err := url.Parse(input.URL)
	if err != nil {
		os.Exit(2)
	}
	u.Fragment = ""
	query := u.Query()
	for _, key := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"} {
		query.Del(key)
	}
	u.RawQuery = query.Encode()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"urls": []string{u.String()}})
}
