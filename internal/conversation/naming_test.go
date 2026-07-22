package conversation

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{"k8s", "my-chat", "what-is-k8s-2", "a", "notes.2026", "snake_case", "0", "x1-_.z"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{
		"",
		"-",
		"-leading-dash",
		".hidden",
		"+",
		"Has-Upper",
		"with space",
		"path/traversal",
		"../evil",
		strings.Repeat("a", 65),
	}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", name)
		}
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"what is kubernetes?", "what-is-kubernetes"},
		{"What IS Kubernetes?!", "what-is-kubernetes"},
		{"explain the difference between TCP and UDP", "explain-the-difference"},
		{"supercalifragilisticexpialidocious rocks", "supercalifragilisticexpi"},
		{"???!!!", "conversation"},
		{"", "conversation"},
		{"  hi  ", "hi"},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
