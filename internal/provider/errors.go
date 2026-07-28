package provider

import (
	"errors"
	"fmt"
	"net"
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

// APIError represents a structured error response from a provider's API.
// Each provider translates its SDK-specific error into this common type,
// keeping SDK knowledge at the provider boundary.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// humanize translates a raw error into a concise, user-readable message.
// It handles infrastructure-level errors (network, DNS, timeouts) that are
// common across all providers. Provider-specific API errors should already
// be translated to *APIError before reaching this function.
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

	return err.Error()
}
