package urlpolicy

import (
	"fmt"
	"net/url"
)

// ParseHTTP validates a remote HTTP(S) endpoint without silently discarding
// credentials or fragments. Query parameters are only allowed for document
// endpoints, never for API base URLs.
func ParseHTTP(value string, allowQuery bool) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("must not contain embedded credentials")
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("must not contain a fragment")
	}
	if !allowQuery && parsed.RawQuery != "" {
		return nil, fmt.Errorf("must not contain a query")
	}
	return parsed, nil
}
