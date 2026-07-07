package provider

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
)

// httpStatusError is a test helper that implements the statusCoder interface.
type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string   { return fmt.Sprintf("HTTP %d", e.code) }
func (e *httpStatusError) StatusCode() int { return e.code }

func TestOpError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *OpError
		want string
	}{
		{
			name: "connection refused",
			err: &OpError{
				Provider: "ollama",
				Op:       "complete",
				Err: &net.OpError{
					Op:  "dial",
					Net: "tcp",
					Addr: &net.TCPAddr{
						IP:   net.IPv4(127, 0, 0, 1),
						Port: 11434,
					},
					Err: &net.OpError{
						Op:  "connect",
						Net: "tcp",
						Err: syscall.ECONNREFUSED,
					},
				},
			},
			want: "ollama: complete: connection refused (127.0.0.1:11434)",
		},
		{
			name: "dns error",
			err: &OpError{
				Provider: "openai",
				Op:       "complete",
				Err: &net.DNSError{
					Err:  "no such host",
					Name: "api.openai.com",
				},
			},
			want: "openai: complete: cannot resolve api.openai.com",
		},
		{
			name: "sdk 401 error string",
			err: &OpError{
				Provider: "anthropic",
				Op:       "complete",
				Err:      fmt.Errorf("POST \"https://api.anthropic.com/v1/messages\": 401 Unauthorized {\"error\":{\"message\":\"invalid api key\"}}"),
			},
			want: "anthropic: complete: unauthorized (check API key)",
		},
		{
			name: "sdk 429 error string",
			err: &OpError{
				Provider: "openai",
				Op:       "stream",
				Err:      fmt.Errorf("POST \"https://api.openai.com/v1/chat/completions\": 429 Too Many Requests {\"error\":{}}"),
			},
			want: "openai: stream: rate limited (try again shortly)",
		},
		{
			name: "bedrock HTTP 403",
			err: &OpError{
				Provider: "bedrock",
				Op:       "complete",
				Err:      &httpStatusError{403},
			},
			want: "bedrock: complete: forbidden (check API key permissions)",
		},
		{
			name: "unknown error passes through",
			err: &OpError{
				Provider: "bedrock",
				Op:       "stream",
				Err:      errors.New("something unexpected"),
			},
			want: "bedrock: stream: something unexpected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpError_Unwrap(t *testing.T) {
	inner := errors.New("root cause")
	err := &OpError{Provider: "openai", Op: "complete", Err: inner}

	if !errors.Is(err, inner) {
		t.Error("errors.Is should find the inner error through Unwrap")
	}
}

func TestExtractHTTPStatus(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{`POST "https://api.openai.com/v1/chat/completions": 401 Unauthorized {"error":{}}`, 401},
		{`POST "https://api.anthropic.com/v1/messages": 529 Overloaded {"error":{}}`, 529},
		{"no status here", 0},
		{`GET "https://example.com": 200 OK`, 200},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.want), func(t *testing.T) {
			got := extractHTTPStatus(tt.input)
			if got != tt.want {
				t.Errorf("extractHTTPStatus(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
