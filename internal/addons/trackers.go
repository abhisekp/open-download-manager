package addons

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

func validTrackerSuggestion(raw string) bool {
	if raw == "" || len(raw) > 2048 || strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "udp") {
		return false
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return false
		}
	} else if u.Scheme == "udp" {
		return false
	}
	return true
}

func decodeSuggestions(raw []byte, m Manifest) ([]string, error) {
	var output map[string]json.RawMessage
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 256<<10 || d.Decode(&output) != nil || d.Decode(new(any)) != io.EOF || len(output) != 1 {
		return nil, errors.New("invalid add-on response")
	}
	field, limit := "urls", 50
	if m.Capability == "tracker-provider" {
		field, limit = "trackers", 128
	}
	var values []string
	if json.Unmarshal(output[field], &values) != nil || values == nil || len(values) > limit || (field == "urls" && len(values) == 0) {
		return nil, errors.New("invalid add-on response")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if m.Capability == "tracker-provider" {
			if !validTrackerSuggestion(value) {
				return nil, errors.New("invalid tracker suggestion; no suggestions accepted")
			}
		} else if !permitted(value, m.OutputHosts) {
			return nil, errors.New("resolved URL is outside approved hosts")
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}
