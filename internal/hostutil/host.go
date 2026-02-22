package hostutil

import (
	"fmt"
	"net/url"
	"strings"
)

const DefaultHost = "api.bitrise.io"

func Normalize(input string) (scheme string, host string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "https", DefaultHost, nil
	}

	var parsed *url.URL
	if strings.Contains(input, "://") {
		parsed, err = url.Parse(input)
		if err != nil {
			return "", "", fmt.Errorf("parse host %q: %w", input, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return "", "", fmt.Errorf("invalid host %q", input)
		}
	} else {
		parsed, err = url.Parse("https://" + input)
		if err != nil {
			return "", "", fmt.Errorf("parse host %q: %w", input, err)
		}
	}

	scheme = strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme == "" {
		scheme = "https"
	}

	host = strings.TrimSpace(parsed.Host)
	if host == "" {
		return "", "", fmt.Errorf("invalid host %q", input)
	}

	return scheme, host, nil
}

func BaseURL(input string) (baseURL string, host string, err error) {
	scheme, host, err := Normalize(input)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%s://%s", scheme, host), host, nil
}
