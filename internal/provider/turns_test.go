package provider

import "testing"

func TestRequestTurns(t *testing.T) {
	// Single-shot form: System/User normalize to one user turn.
	system, turns := Request{System: "sys", User: "hello"}.Turns()
	if system != "sys" {
		t.Errorf("single-shot system = %q, want sys", system)
	}
	if len(turns) != 1 || turns[0].Role != "user" || turns[0].Content != "hello" {
		t.Errorf("single-shot turns = %+v", turns)
	}

	// Single-shot without a system prompt.
	system, _ = Request{User: "hello"}.Turns()
	if system != "" {
		t.Errorf("system = %q, want empty", system)
	}

	// Multi-turn form takes precedence over System/User.
	system, turns = Request{
		System: "IGNORED", User: "IGNORED",
		Messages: []Message{
			{Role: "system", Content: "be brief"},
			{Role: "user", Content: "what is k8s?"},
			{Role: "assistant", Content: "an orchestrator"},
			{Role: "user", Content: "and swarm?"},
		},
	}.Turns()
	if system != "be brief" {
		t.Errorf("multi-turn system = %q, want 'be brief'", system)
	}
	if len(turns) != 3 {
		t.Fatalf("got %d turns, want 3 (system lifted out)", len(turns))
	}
	if turns[1].Role != "assistant" || turns[2].Content != "and swarm?" {
		t.Errorf("turns = %+v", turns)
	}
}
