package conversation

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func testDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)
	return dir
}

func TestDirResolution(t *testing.T) {
	t.Setenv("CLAI_CONVERSATIONS_DIR", "/explicit/override")
	if dir, _ := Dir(); dir != "/explicit/override" {
		t.Errorf("CLAI_CONVERSATIONS_DIR not honored: %q", dir)
	}
	t.Setenv("CLAI_CONVERSATIONS_DIR", "")
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	if dir, _ := Dir(); dir != filepath.Join("/xdg/state", "clai", "conversations") {
		t.Errorf("XDG_STATE_HOME not honored: %q", dir)
	}
	t.Setenv("XDG_STATE_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "state", "clai", "conversations")
	if dir, _ := Dir(); dir != want {
		t.Errorf("default = %q, want %q", dir, want)
	}
}

func TestAppendAndMessagesRoundtrip(t *testing.T) {
	testDir(t)
	c, err := Open("roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	in := []Message{
		{Role: "system", Content: "be brief", Model: "openai/gpt-4.1", TS: ts},
		{Role: "user", Content: "what is k8s?", TS: ts},
		{Role: "assistant", Content: "an orchestrator", TS: ts, TokensIn: 25, TokensOut: 150},
	}
	for _, m := range in {
		if err := c.Append(m); err != nil {
			t.Fatal(err)
		}
	}
	got, skipped, err := c.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3", len(got))
	}
	if got[0].Model != "openai/gpt-4.1" || got[2].TokensOut != 150 {
		t.Errorf("fields lost in roundtrip: %+v", got)
	}
}

func TestMessagesSkipsMalformedLines(t *testing.T) {
	dir := testDir(t)
	content := "{\"role\":\"user\",\"content\":\"ok\",\"ts\":\"2026-07-17T10:00:00Z\"}\n{not json\n{\"role\":\"assistant\",\"content\":\"also ok\",\"ts\":\"2026-07-17T10:00:01Z\"}\n"
	if err := os.WriteFile(filepath.Join(dir, "torn.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, _ := Open("torn")
	got, skipped, _ := c.Messages()
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if len(got) != 2 {
		t.Errorf("got %d messages, want 2", len(got))
	}
}

func TestMessagesOnMissingFileIsEmpty(t *testing.T) {
	testDir(t)
	c, _ := Open("never-written")
	got, skipped, err := c.Messages()
	if err != nil || skipped != 0 || len(got) != 0 {
		t.Errorf("want empty/0/nil, got %v/%d/%v", got, skipped, err)
	}
}

func TestNewDeduplicatesSlugs(t *testing.T) {
	testDir(t)
	c1, _ := New("what is kubernetes?")
	if c1.Name != "what-is-kubernetes" {
		t.Fatalf("first name = %q", c1.Name)
	}
	if !c1.IsNew() {
		t.Error("New() should be IsNew")
	}
	c2, _ := New("what is kubernetes?")
	if c2.Name != "what-is-kubernetes-2" {
		t.Errorf("second = %q", c2.Name)
	}
	c3, _ := New("what is kubernetes?")
	if c3.Name != "what-is-kubernetes-3" {
		t.Errorf("third = %q", c3.Name)
	}
}

func TestLatestByMtime(t *testing.T) {
	dir := testDir(t)
	for _, name := range []string{"older", "newest"} {
		c, _ := Open(name)
		_ = c.Append(Message{Role: "user", Content: "hi", TS: time.Now()})
	}
	past := time.Now().Add(-1 * time.Hour)
	_ = os.Chtimes(filepath.Join(dir, "older.jsonl"), past, past)
	c, _ := Latest()
	if c.Name != "newest" {
		t.Errorf("Latest() = %q, want newest", c.Name)
	}
}

func TestLatestEmptyDirErrors(t *testing.T) {
	testDir(t)
	if _, err := Latest(); err == nil {
		t.Error("Latest() on empty dir should error")
	}
}

func TestReadPathsDoNotCreateDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never-created")
	t.Setenv("CLAI_CONVERSATIONS_DIR", missing)
	if sums, err := List(); err != nil || len(sums) != 0 {
		t.Errorf("List() on missing dir = (%v, %v)", sums, err)
	}
	if _, err := Latest(); err == nil {
		t.Error("Latest() on missing dir should error")
	}
	if _, err := os.Stat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("read paths created directory: %v", err)
	}
}

func TestListSummaries(t *testing.T) {
	testDir(t)
	c, _ := Open("k8s")
	ts := time.Now()
	_ = c.Append(Message{Role: "system", Content: "be brief", Model: "openai/gpt-4.1", TS: ts})
	_ = c.Append(Message{Role: "user", Content: "what is kubernetes and why does everyone keep saying it is complicated", TS: ts})
	sums, _ := List()
	if len(sums) != 1 {
		t.Fatalf("got %d, want 1", len(sums))
	}
	if sums[0].Name != "k8s" || sums[0].Model != "openai/gpt-4.1" {
		t.Errorf("sum = %+v", sums[0])
	}
	if len(sums[0].Preview) > 48 || !strings.HasPrefix(sums[0].Preview, "what is kubernetes") {
		t.Errorf("preview = %q", sums[0].Preview)
	}
}

func TestRename(t *testing.T) {
	testDir(t)
	c, _ := Open("old-name")
	_ = c.Append(Message{Role: "user", Content: "hi", TS: time.Now()})
	if err := Rename("old-name", "new-name"); err != nil {
		t.Fatal(err)
	}
	if err := Rename("old-name", "elsewhere"); err == nil {
		t.Error("renaming missing should error")
	}
	c2, _ := Open("blocker")
	_ = c2.Append(Message{Role: "user", Content: "x", TS: time.Now()})
	if err := Rename("new-name", "blocker"); err == nil {
		t.Error("renaming onto existing should error")
	}
	if err := Rename("new-name", "-invalid"); err == nil {
		t.Error("renaming to invalid should error")
	}
}

func TestRemove(t *testing.T) {
	testDir(t)
	c, _ := Open("doomed")
	_ = c.Append(Message{Role: "user", Content: "hi", TS: time.Now()})
	if err := Remove("doomed"); err != nil {
		t.Fatal(err)
	}
	if err := Remove("doomed"); err == nil {
		t.Error("second remove should error")
	}
}

func TestRemoveOlderThan(t *testing.T) {
	dir := testDir(t)
	for _, name := range []string{"ancient", "recent"} {
		c, _ := Open(name)
		_ = c.Append(Message{Role: "user", Content: "hi", TS: time.Now()})
	}
	old := time.Now().Add(-40 * 24 * time.Hour)
	_ = os.Chtimes(filepath.Join(dir, "ancient.jsonl"), old, old)
	n, _ := RemoveOlderThan(30 * 24 * time.Hour)
	if n != 1 {
		t.Errorf("removed %d, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "recent.jsonl")); err != nil {
		t.Error("recent should survive")
	}
}

func TestOpenReportsNew(t *testing.T) {
	testDir(t)
	c, err := Open("fresh")
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsNew() {
		t.Error("Open on a missing file should report IsNew")
	}
	if err := c.Append(Message{Role: "user", Content: "hi", TS: time.Now()}); err != nil {
		t.Fatal(err)
	}
	again, err := Open("fresh")
	if err != nil {
		t.Fatal(err)
	}
	if again.IsNew() {
		t.Error("Open on an existing file must not report IsNew")
	}
}

func TestRemoveAndRenameValidateNames(t *testing.T) {
	dir := testDir(t)

	// A real file one level above the conversations dir: traversal must not reach it.
	outside := filepath.Join(filepath.Dir(dir), "victim.jsonl")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Remove("../victim"); err == nil {
		t.Error("Remove must reject invalid names")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("traversal removed a file outside the conversations dir")
	}
	if err := Rename("../victim", "ok-name"); err == nil {
		t.Error("Rename must reject an invalid old name")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("traversal renamed a file outside the conversations dir")
	}
}

func TestFirstUserPreviewIsRuneSafe(t *testing.T) {
	// One long multibyte word with no spaces: a byte-based cut at 48 lands
	// mid-rune ("a" + 3-byte runes means byte 48 splits a rune).
	long := "a" + strings.Repeat("€", 40)
	got := firstUserPreview([]Message{{Role: "user", Content: long}})
	if !utf8.ValidString(got) {
		t.Errorf("preview is not valid UTF-8: %q", got)
	}
	if n := len([]rune(got)); n > 48 {
		t.Errorf("preview too long: %d runes", n)
	}
}
