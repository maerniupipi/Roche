package utils

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateAllowedRedirectURI rejects open redirects by requiring the requested
// post-login URI to exactly match an operator-managed allowlist. Relative paths
// are supported for same-origin deployments, but scheme-relative URLs are not.
func ValidateAllowedRedirectURI(rawURI, allowedCSV string) error {
	normalized, err := normalizeRedirectURI(rawURI)
	if err != nil {
		return err
	}

	if strings.TrimSpace(allowedCSV) == "" {
		return fmt.Errorf("redirect URI allowlist is not configured")
	}
	for _, candidate := range strings.Split(allowedCSV, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		allowed, candidateErr := normalizeRedirectURI(candidate)
		if candidateErr != nil {
			continue
		}
		if normalized == allowed {
			return nil
		}
	}
	return fmt.Errorf("redirect URI is not allowed")
}

func normalizeRedirectURI(rawURI string) (string, error) {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return "", fmt.Errorf("redirect URI is required")
	}
	if len(rawURI) > 2048 {
		return "", fmt.Errorf("redirect URI exceeds maximum length")
	}

	parsed, err := url.Parse(rawURI)
	if err != nil {
		return "", fmt.Errorf("invalid redirect URI: %w", err)
	}
	if parsed.Opaque != "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("redirect URI contains unsupported components")
	}

	if parsed.IsAbs() {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", fmt.Errorf("redirect URI must use http or https")
		}
		if parsed.Hostname() == "" {
			return "", fmt.Errorf("redirect URI hostname is required")
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		if parsed.Path == "" {
			parsed.Path = "/"
		}
		return parsed.String(), nil
	}

	if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(rawURI, "//") {
		return "", fmt.Errorf("relative redirect URI must be an absolute path")
	}
	return parsed.String(), nil
}
