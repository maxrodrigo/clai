package strategy

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/maxrodrigo/clai/internal/testsupport"
)

func TestMain(m *testing.M) {
	// Initialize the strategy package with the in-repo data directory so tests
	// are hermetic and do not depend on CLAI_DATA_DIR or an installed copy.
	Init(testsupport.DataDir(), "", nil)
	os.Exit(m.Run())
}

func TestApply(t *testing.T) {
	s := &Strategy{Name: "cot", Prompt: "Think step by step."}
	system := "You are a helpful assistant."
	got := s.Apply(system)
	if !strings.HasPrefix(got, "Think step by step.") {
		t.Errorf("Apply() should prepend strategy, got %q", got)
	}
	if !strings.Contains(got, system) {
		t.Errorf("Apply() should contain system prompt, got %q", got)
	}
}

func TestNilApply(t *testing.T) {
	var s *Strategy
	system := "You are a helpful assistant."
	if got := s.Apply(system); got != system {
		t.Errorf("nil Strategy.Apply() = %q, want %q", got, system)
	}
}

func TestResolve_NotFoundError(t *testing.T) {
	_, err := Resolve("nonexistent-strategy-xyz")
	if err == nil {
		t.Fatal("Resolve() expected error, got nil")
	}

	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("error should be *NotFoundError, got %T: %v", err, err)
	}
	if nfe.Name != "nonexistent-strategy-xyz" {
		t.Errorf("NotFoundError.Name = %q, want %q", nfe.Name, "nonexistent-strategy-xyz")
	}
}

func TestResolve_systemStrategy(t *testing.T) {
	// "cot" is a system strategy.
	s, err := Resolve("cot")
	if err != nil {
		t.Fatalf("Resolve(\"cot\") error: %v", err)
	}
	if s == nil {
		t.Fatal("Resolve(\"cot\") = nil, want non-nil")
	}
	if s.Name != "cot" {
		t.Errorf("Name = %q, want %q", s.Name, "cot")
	}
	if s.Prompt == "" {
		t.Error("Prompt is empty, want non-empty")
	}
}

func TestResolve_none(t *testing.T) {
	s, err := Resolve("none")
	if err != nil {
		t.Fatalf("Resolve(\"none\") error: %v", err)
	}
	if s != nil {
		t.Errorf("Resolve(\"none\") = %v, want nil", s)
	}
}

func TestResolve_empty(t *testing.T) {
	s, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\") error: %v", err)
	}
	if s != nil {
		t.Errorf("Resolve(\"\") = %v, want nil", s)
	}
}

func TestList_includesSystem(t *testing.T) {
	strategies, err := List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(strategies) == 0 {
		t.Fatal("List() = empty, want at least system strategies")
	}
	names := make(map[string]bool, len(strategies))
	for _, s := range strategies {
		names[s.Name] = true
	}
	for _, want := range []string{"cot", "cod", "self-refine"} {
		if !names[want] {
			t.Errorf("List() missing system strategy %q", want)
		}
	}
}

func TestParse_withTitle(t *testing.T) {
	data := []byte("# Chain of Thought\n\nThink step by step.\n")
	s := parse("cot", "(test)", data)
	if s.Description != "Chain of Thought" {
		t.Errorf("Description = %q, want %q", s.Description, "Chain of Thought")
	}
	if s.Prompt != "Think step by step." {
		t.Errorf("Prompt = %q, want %q", s.Prompt, "Think step by step.")
	}
}

func TestParse_withoutTitle(t *testing.T) {
	data := []byte("Think step by step.\n")
	s := parse("cot", "(test)", data)
	// No # title means description defaults to name.
	if s.Description != "cot" {
		t.Errorf("Description = %q, want %q (name fallback)", s.Description, "cot")
	}
	if s.Prompt != "Think step by step." {
		t.Errorf("Prompt = %q, want %q", s.Prompt, "Think step by step.")
	}
}
