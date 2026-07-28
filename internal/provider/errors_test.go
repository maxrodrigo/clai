package provider

import (
	"errors"
	"net"
	"syscall"
	"testing"
)

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
			name: "api error with message",
			err: &OpError{
				Provider: "anthropic",
				Op:       "stream",
				Err:      &APIError{StatusCode: 400, Message: "max_tokens: 8192 exceeds model maximum"},
			},
			want: "anthropic: stream: max_tokens: 8192 exceeds model maximum (HTTP 400)",
		},
		{
			name: "api error without message",
			err: &OpError{
				Provider: "bedrock",
				Op:       "complete",
				Err:      &APIError{StatusCode: 503},
			},
			want: "bedrock: complete: HTTP 503",
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

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "with message",
			err:  &APIError{StatusCode: 400, Message: "invalid model"},
			want: "invalid model (HTTP 400)",
		},
		{
			name: "without message",
			err:  &APIError{StatusCode: 401},
			want: "HTTP 401",
		},
		{
			name: "rate limit with message",
			err:  &APIError{StatusCode: 429, Message: "rate limit exceeded, try again in 30s"},
			want: "rate limit exceeded, try again in 30s (HTTP 429)",
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
