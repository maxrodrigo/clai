package provider

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// OpError represents a failed provider operation. It follows the same pattern
// as *net.OpError and *url.Error in the standard library: the outer type
// carries what this layer knows (provider, operation), and the inner Err holds
// the underlying cause. The Error() method produces a clean, user-facing
// message while Unwrap() preserves the full chain for programmatic inspection.
type OpError struct {
	Provider string // "openai", "bedrock", "anthropic"
	Op       string // "complete", "stream", "list models"
	Err      error  // underlying cause
}

func (e *OpError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Provider, e.Op, humanize(e.Err))
}

func (e *OpError) Unwrap() error { return e.Err }

// humanize translates a raw error into a concise, user-readable message.
// It inspects the error chain for known types (network errors, HTTP status
// errors from SDKs) and extracts the essential information.
func humanize(err error) string {
	if err == nil {
		return ""
	}

	// Connection refused / network unreachable / DNS failure.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		addr := opErr.Addr
		if addr == nil {
			addr = opErr.Source
		}
		host := ""
		if addr != nil {
			host = addr.String()
		}
		// Unwrap to the deepest syscall message.
		inner := opErr.Err
		for u := errors.Unwrap(inner); u != nil; u = errors.Unwrap(inner) {
			inner = u
		}
		msg := inner.Error()
		if host != "" {
			return fmt.Sprintf("%s (%s)", msg, host)
		}
		return msg
	}

	// DNS resolution failure.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Sprintf("cannot resolve %s", dnsErr.Name)
	}

	// Timeout (net.Error interface).
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "request timed out"
	}

	// SDK HTTP errors: both openai-go and anthropic-sdk-go produce errors
	// whose Error() string starts with the HTTP method and contains the
	// status code. They also expose a StatusCode field. We detect them via
	// a lightweight interface rather than importing the SDK packages.
	type statusCoder interface {
		error
		StatusCode() int
	}

	var sc statusCoder
	if errors.As(err, &sc) {
		return httpMessage(sc.StatusCode())
	}

	// The Stainless SDKs (openai-go, anthropic-sdk-go) store StatusCode as a
	// public field, not a method. We can't use an interface for that, but the
	// Error() string always starts with "POST/GET ... : NNN StatusText".
	// Extract the status code from the string as a fallback.
	if code := extractHTTPStatus(err.Error()); code > 0 {
		return httpMessage(code)
	}

	// Fallback: return the original error string.
	return err.Error()
}

// httpMessage returns a concise description for common HTTP status codes.
func httpMessage(code int) string {
	switch code {
	case 401:
		return "unauthorized (check API key)"
	case 403:
		return "forbidden (check API key permissions)"
	case 404:
		return "model not found"
	case 429:
		return "rate limited (try again shortly)"
	case 500:
		return "server error (provider issue)"
	case 502, 503:
		return "service unavailable (try again shortly)"
	default:
		return fmt.Sprintf("HTTP %d", code)
	}
}

// extractHTTPStatus looks for a "NNN StatusText" pattern in an error string
// produced by the Stainless-generated SDKs. These errors look like:
//
//	POST "https://api.openai.com/v1/...": 401 Unauthorized {...}
//
// Returns 0 if no status code is found.
func extractHTTPStatus(s string) int {
	idx := strings.Index(s, "\": ")
	if idx < 0 {
		return 0
	}
	rest := s[idx+3:] // skip past "\": "
	if len(rest) < 3 {
		return 0
	}
	// A status code is exactly three digits, terminated by end-of-string or a
	// space before the status text.
	if len(rest) > 3 && rest[3] != ' ' {
		return 0
	}
	code, err := strconv.Atoi(rest[:3])
	if err != nil || code < 100 || code > 599 {
		return 0
	}
	return code
}
