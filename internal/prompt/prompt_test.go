package prompt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxrodrigo/clai/internal/config"
)

func TestSplitFrontmatter(t *testing.T) {
	input := "---\nmodel: openai/gpt-4o-mini\ntemperature: 0.3\ndescription: Test prompt\n---\nYou are a helpful assistant.\n"
	fm, body, err := splitFrontmatter([]byte(input))
	if err != nil {
		t.Fatalf("splitFrontmatter() error: %v", err)
	}
	if got, want := fm.Model, "openai/gpt-4o-mini"; got != want {
		t.Errorf("Model = %q, want %q", got, want)
	}
	if fm.Temperature == nil {
		t.Fatal("Temperature = nil, want *0.3")
	}
	if got, want := *fm.Temperature, 0.3; got != want {
		t.Errorf("Temperature = %v, want %v", got, want)
	}
	if got, want := fm.Description, "Test prompt"; got != want {
		t.Errorf("Description = %q, want %q", got, want)
	}
	if !strings.Contains(body, "You are a helpful assistant") {
		t.Errorf("body missing expected text: %q", body)
	}
}

func TestSplitFrontmatterNone(t *testing.T) {
	input := "You are a helpful assistant.\n"
	fm, body, err := splitFrontmatter([]byte(input))
	if err != nil {
		t.Fatalf("splitFrontmatter() error: %v", err)
	}
	if fm.Model != "" || fm.Temperature != nil {
		t.Errorf("expected empty frontmatter, got %+v", fm)
	}
	if got, want := body, input; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestMergeDefaults(t *testing.T) {
	temp := func(v float64) *float64 { return &v }

	t.Run("applies when unset", func(t *testing.T) {
		cfg := &config.Config{} // empty config
		fm := Frontmatter{Model: "openai/gpt-4o-mini", Temperature: temp(0.5)}
		merged := MergeDefaults(cfg, fm)

		if got, want := merged.Model, "openai/gpt-4o-mini"; got != want {
			t.Errorf("Model = %q, want %q", got, want)
		}
		if merged.Temperature == nil || *merged.Temperature != 0.5 {
			t.Errorf("Temperature = %v, want 0.5", merged.Temperature)
		}
	})

	t.Run("applies explicit zero temperature", func(t *testing.T) {
		// temperature: 0 in frontmatter must be applied, not silently ignored.
		cfg := &config.Config{}
		fm := Frontmatter{Temperature: temp(0.0)}
		merged := MergeDefaults(cfg, fm)

		if merged.Temperature == nil || *merged.Temperature != 0.0 {
			t.Errorf("Temperature = %v, want 0.0 (explicit zero must not be ignored)", merged.Temperature)
		}
	})

	t.Run("does not override user config", func(t *testing.T) {
		cfg := &config.Config{
			Model: "anthropic/claude-3-7-sonnet-20250219",
		}
		merged := MergeDefaults(cfg, Frontmatter{Model: "openai/gpt-4o-mini"})
		if got, want := merged.Model, "anthropic/claude-3-7-sonnet-20250219"; got != want {
			t.Errorf("Model = %q, want %q (user config should not be overridden)", got, want)
		}
	})
}

func TestParsePrepend(t *testing.T) {
	dir := t.TempDir()

	// Create a voice file to prepend.
	voiceContent := "Write in short, direct sentences. No filler."
	voicePath := filepath.Join(dir, "voice.md")
	if err := os.WriteFile(voicePath, []byte(voiceContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a prompt that references the voice file.
	promptContent := "---\nprepend:\n  - voice.md\n---\nSummarize the following text.\n"
	promptPath := filepath.Join(dir, "summarize.md")

	p, err := Parse("summarize", promptPath, []byte(promptContent))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	want := voiceContent + "\n\n" + "Summarize the following text."
	if got := p.Content; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParseAppend(t *testing.T) {
	dir := t.TempDir()

	// Create a constraints file to append.
	constraintsContent := "Never use bullet points."
	constraintsPath := filepath.Join(dir, "constraints.md")
	if err := os.WriteFile(constraintsPath, []byte(constraintsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	promptContent := "---\nappend:\n  - constraints.md\n---\nSummarize the following text.\n"
	promptPath := filepath.Join(dir, "summarize.md")

	p, err := Parse("summarize", promptPath, []byte(promptContent))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	want := "Summarize the following text." + "\n\n" + constraintsContent
	if got := p.Content; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParsePrependAndAppend(t *testing.T) {
	dir := t.TempDir()

	voiceContent := "Be concise."
	if err := os.WriteFile(filepath.Join(dir, "voice.md"), []byte(voiceContent), 0o644); err != nil {
		t.Fatal(err)
	}

	constraintsContent := "Output plain text only."
	if err := os.WriteFile(filepath.Join(dir, "constraints.md"), []byte(constraintsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	promptContent := "---\nprepend:\n  - voice.md\nappend:\n  - constraints.md\n---\nSummarize the following text.\n"
	promptPath := filepath.Join(dir, "summarize.md")

	p, err := Parse("summarize", promptPath, []byte(promptContent))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	want := voiceContent + "\n\n" + "Summarize the following text." + "\n\n" + constraintsContent
	if got := p.Content; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParseMultiplePrepend(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("First."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("Second."), 0o644); err != nil {
		t.Fatal(err)
	}

	promptContent := "---\nprepend:\n  - a.md\n  - b.md\n---\nBody.\n"
	promptPath := filepath.Join(dir, "test.md")

	p, err := Parse("test", promptPath, []byte(promptContent))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	want := "First.\n\nSecond.\n\nBody."
	if got := p.Content; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParsePrependAbsolutePath(t *testing.T) {
	dir := t.TempDir()

	voiceContent := "Absolute voice."
	voicePath := filepath.Join(dir, "voice.md")
	if err := os.WriteFile(voicePath, []byte(voiceContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Prompt in a different directory references the voice file by absolute path.
	promptDir := t.TempDir()
	promptContent := "---\nprepend:\n  - " + voicePath + "\n---\nBody.\n"
	promptPath := filepath.Join(promptDir, "test.md")

	p, err := Parse("test", promptPath, []byte(promptContent))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	want := voiceContent + "\n\n" + "Body."
	if got := p.Content; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParsePrependMissingFile(t *testing.T) {
	dir := t.TempDir()

	promptContent := "---\nprepend:\n  - nonexistent.md\n---\nBody.\n"
	promptPath := filepath.Join(dir, "test.md")

	_, err := Parse("test", promptPath, []byte(promptContent))
	if err == nil {
		t.Fatal("Parse() expected error for missing prepend file, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent.md") {
		t.Errorf("error should mention the missing file, got: %v", err)
	}
}

func TestParseNoPrependAppend(t *testing.T) {
	dir := t.TempDir()

	// Prompt with no prepend/append should work as before.
	promptContent := "---\nmodel: openai/gpt-4o\n---\nJust a prompt.\n"
	promptPath := filepath.Join(dir, "test.md")

	p, err := Parse("test", promptPath, []byte(promptContent))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if got, want := p.Content, "Just a prompt."; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestExtendsBasic(t *testing.T) {
	resetSources()
	defer resetSources()

	dir := t.TempDir()

	// Create base prompt
	baseContent := "---\nmodel: openai/gpt-4o\ntemperature: 0.7\n---\nBase body."
	if err := os.WriteFile(filepath.Join(dir, "base.md"), []byte(baseContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create child prompt that extends base
	childContent := "---\nextends: base\ndescription: Child prompt\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "child.md"), []byte(childContent), 0o644); err != nil {
		t.Fatal(err)
	}

	registerSource(newFilesystemSource("test", dir, nil))

	p, err := Resolve("child")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Should inherit base's body
	if got, want := p.Content, "Base body."; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}

	// Should inherit base's model
	if got, want := p.Frontmatter.Model, "openai/gpt-4o"; got != want {
		t.Errorf("Model = %q, want %q", got, want)
	}

	// Should have child's description
	if got, want := p.Frontmatter.Description, "Child prompt"; got != want {
		t.Errorf("Description = %q, want %q", got, want)
	}
}

func TestExtendsSameNameRejected(t *testing.T) {
	resetSources()
	defer resetSources()

	dir := t.TempDir()

	// Create a prompt that tries to extend itself by name
	content := "---\nextends: tweet\ndescription: My custom tweet\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "tweet.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	registerSource(newFilesystemSource("test", dir, nil))

	_, err := Resolve("tweet")
	if err == nil {
		t.Fatal("Resolve() should error when prompt extends itself by name")
	}
	if !strings.Contains(err.Error(), "cannot extend itself") {
		t.Errorf("error should mention 'cannot extend itself', got: %v", err)
	}
}

func TestExtendsWithPrepend(t *testing.T) {
	resetSources()
	defer resetSources()

	dir := t.TempDir()

	// Create voice file
	voiceContent := "Write in my voice."
	if err := os.WriteFile(filepath.Join(dir, "voice.md"), []byte(voiceContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create base prompt
	baseContent := "---\ntemperature: 0.5\n---\nBase body."
	if err := os.WriteFile(filepath.Join(dir, "base.md"), []byte(baseContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create child that extends base and prepends voice
	childContent := "---\nextends: base\nprepend:\n  - voice.md\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "child.md"), []byte(childContent), 0o644); err != nil {
		t.Fatal(err)
	}

	registerSource(newFilesystemSource("test", dir, nil))

	p, err := Resolve("child")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Should have voice prepended to base body
	want := "Write in my voice.\n\nBase body."
	if got := p.Content; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestExtendsCircularDetection(t *testing.T) {
	resetSources()
	defer resetSources()

	dir := t.TempDir()

	// Create circular: a -> b -> a
	aContent := "---\nextends: b\n---\nA body."
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(aContent), 0o644); err != nil {
		t.Fatal(err)
	}

	bContent := "---\nextends: a\n---\nB body."
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte(bContent), 0o644); err != nil {
		t.Fatal(err)
	}

	registerSource(newFilesystemSource("test", dir, nil))

	_, err := Resolve("a")
	if err == nil {
		t.Fatal("Resolve() should error on circular extends")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("error should mention circular, got: %v", err)
	}
}

func TestIsValidName(t *testing.T) {
	valid := []string{"review", "my-prompt", "my_prompt", "a", "review2"}
	for _, name := range valid {
		if !IsValidName(name) {
			t.Errorf("IsValidName(%q) = false, want true", name)
		}
	}

	invalid := []string{"", "-bad", "alice/review", "has space", "has/slash/two"}
	for _, name := range invalid {
		if IsValidName(name) {
			t.Errorf("IsValidName(%q) = true, want false", name)
		}
	}
}

func TestIsNamespaced(t *testing.T) {
	valid := []string{"alice/review", "bob/my-prompt", "org_name/prompt_name"}
	for _, name := range valid {
		if !IsNamespaced(name) {
			t.Errorf("IsNamespaced(%q) = false, want true", name)
		}
	}

	invalid := []string{"review", "", "/review", "alice/", "alice/review/extra", "-bad/review"}
	for _, name := range invalid {
		if IsNamespaced(name) {
			t.Errorf("IsNamespaced(%q) = true, want false", name)
		}
	}
}

func TestParseNamespace(t *testing.T) {
	owner, name := ParseNamespace("alice/review")
	if owner != "alice" || name != "review" {
		t.Errorf("ParseNamespace(%q) = (%q, %q), want (\"alice\", \"review\")", "alice/review", owner, name)
	}

	owner, name = ParseNamespace("bare")
	if owner != "" || name != "bare" {
		t.Errorf("ParseNamespace(%q) = (%q, %q), want (\"\", \"bare\")", "bare", owner, name)
	}
}

func TestCommunitySourceResolve(t *testing.T) {
	resetSources()
	defer resetSources()

	dir := t.TempDir()

	// Create community prompt at dir/alice/review.md
	ownerDir := filepath.Join(dir, "alice")
	if err := os.MkdirAll(ownerDir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\ndescription: Alice's review\n---\nYou are a reviewer.\n"
	if err := os.WriteFile(filepath.Join(ownerDir, "review.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	registerSource(newCommunitySource(dir, nil))

	p, err := Resolve("alice/review")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if p.Name != "alice/review" {
		t.Errorf("Name = %q, want %q", p.Name, "alice/review")
	}
	if p.Frontmatter.Description != "Alice's review" {
		t.Errorf("Description = %q, want %q", p.Frontmatter.Description, "Alice's review")
	}
}

func TestCommunitySourceNotFound(t *testing.T) {
	resetSources()
	defer resetSources()

	dir := t.TempDir()
	registerSource(newCommunitySource(dir, nil))

	_, err := Resolve("alice/nonexistent")
	if err == nil {
		t.Fatal("Resolve() expected error for missing community prompt, got nil")
	}
}

func TestCommunitySourceIgnoresBareNames(t *testing.T) {
	dir := t.TempDir()
	src := newCommunitySource(dir, nil)

	// A bare name should return nil (not found), not an error
	p, err := src.Resolve("review")
	if err != nil {
		t.Fatalf("Resolve() error on bare name: %v", err)
	}
	if p != nil {
		t.Errorf("Resolve() = %v, want nil for bare name", p)
	}
}

func TestCommunitySourceList(t *testing.T) {
	dir := t.TempDir()

	// Create two owners with one prompt each
	for _, tc := range []struct{ owner, name, desc string }{
		{"alice", "review", "Alice review"},
		{"bob", "summarize", "Bob summarize"},
	} {
		ownerDir := filepath.Join(dir, tc.owner)
		if err := os.MkdirAll(ownerDir, 0o750); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf("---\ndescription: %s\n---\nBody.\n", tc.desc)
		if err := os.WriteFile(filepath.Join(ownerDir, tc.name+".md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	src := newCommunitySource(dir, nil)
	prompts, err := src.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(prompts) != 2 {
		t.Errorf("List() returned %d prompts, want 2", len(prompts))
	}

	names := map[string]bool{}
	for _, p := range prompts {
		names[p.Name] = true
	}
	for _, want := range []string{"alice/review", "bob/summarize"} {
		if !names[want] {
			t.Errorf("List() missing %q", want)
		}
	}
}

func TestExtendsChildBodyOverridesBase(t *testing.T) {
	resetSources()
	defer resetSources()

	dir := t.TempDir()

	// Create base prompt
	baseContent := "---\nmodel: openai/gpt-4o\n---\nBase body."
	if err := os.WriteFile(filepath.Join(dir, "base.md"), []byte(baseContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create child with its own body
	childContent := "---\nextends: base\n---\nChild body."
	if err := os.WriteFile(filepath.Join(dir, "child.md"), []byte(childContent), 0o644); err != nil {
		t.Fatal(err)
	}

	registerSource(newFilesystemSource("test", dir, nil))

	p, err := Resolve("child")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Child's body should override base's body
	if got, want := p.Content, "Child body."; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}

	// But still inherit base's model
	if got, want := p.Frontmatter.Model, "openai/gpt-4o"; got != want {
		t.Errorf("Model = %q, want %q", got, want)
	}
}

func TestResolve_NotFoundError(t *testing.T) {
	resetSources()
	defer resetSources()

	dir := t.TempDir()
	registerSource(newFilesystemSource("test", dir, nil))

	_, err := Resolve("nonexistent-xyz")
	if err == nil {
		t.Fatal("Resolve() expected error, got nil")
	}

	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("error should be *NotFoundError, got %T: %v", err, err)
	}
	if nfe.Name != "nonexistent-xyz" {
		t.Errorf("NotFoundError.Name = %q, want %q", nfe.Name, "nonexistent-xyz")
	}
}
