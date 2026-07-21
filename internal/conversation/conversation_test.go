package conversation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

// cleanupTargetEnv names the env var the parent passes to the child subprocess
// containing the absolute path to the pre-created test fixture directory.
// The child validates it rigorously before using it as CLAI_CONVERSATIONS_DIR.
const cleanupTargetEnv = "CLAI_TEST_CLEANUP_TARGET"

// markerFileName is a file created by the parent inside the fixture directory.
// The child requires its presence and correct content before proceeding.
const markerFileName = ".clai-test-marker"
const markerContent = "clai-cleanup-test-fixture"
const fixtureDirPrefix = "clai-cleanup-test-"

// validateCleanupTarget checks the target passed to a subprocess helper is safe.
// It rejects anything that isn't an absolute path, a direct child of os.TempDir,
// matching the expected prefix, and containing the marker file with correct content.
func validateCleanupTarget(t *testing.T) string {
	t.Helper()
	target := os.Getenv(cleanupTargetEnv)
	if target == "" {
		t.Fatal("child: CLAI_TEST_CLEANUP_TARGET not set")
	}
	if !filepath.IsAbs(target) {
		t.Fatalf("child: target %q is not absolute", target)
	}
	// Must be a direct child of the system temp directory.
	parent := filepath.Dir(target)
	tmpDir := os.TempDir()
	cleanParent, _ := filepath.EvalSymlinks(parent)
	cleanTmp, _ := filepath.EvalSymlinks(tmpDir)
	if cleanParent != cleanTmp {
		t.Fatalf("child: target parent %q is not system temp dir %q", parent, tmpDir)
	}
	base := filepath.Base(target)
	if !strings.HasPrefix(base, fixtureDirPrefix) {
		t.Fatalf("child: target basename %q does not start with %q", base, fixtureDirPrefix)
	}
	// Validate marker file.
	marker := filepath.Join(target, markerFileName)
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("child: read marker: %v", err)
	}
	if string(data) != markerContent {
		t.Fatalf("child: marker content = %q, want %q", data, markerContent)
	}
	info, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o444 {
		t.Fatalf("child: marker perms = %o, want 0444", info.Mode().Perm())
	}
	return target
}

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

func TestAppendBatchWritesOneJSONLinePerMessage(t *testing.T) {
	testDir(t)
	c, err := Open("batch")
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	messages := []Message{
		{Role: "user", Content: "question", TS: ts},
		{Role: "assistant", Content: "answer", TS: ts},
	}

	if err := c.Append(messages...); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte{'\n'}) || bytes.Count(data, []byte{'\n'}) != len(messages) {
		t.Fatalf("batch is not exactly one newline-terminated JSON object per message: %q", data)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != len(messages) {
		t.Fatalf("got %d JSON lines, want %d", len(lines), len(messages))
	}
	got, skipped, err := c.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 || len(got) != len(messages) {
		t.Fatalf("Messages() = (%v, %d, %v), want %d messages", got, skipped, err, len(messages))
	}
	for i := range messages {
		if got[i] != messages[i] {
			t.Errorf("message %d = %+v, want %+v", i, got[i], messages[i])
		}
	}
}

func TestAppendEmptyBatchDoesNotCreateFilesystemEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)
	c, err := Open("empty")
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Append(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty append created conversation directory: %v", err)
	}
	if _, err := os.Stat(c.Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty append created conversation file: %v", err)
	}
}

func TestAppendMarshalFailureLeavesExistingFileUnchanged(t *testing.T) {
	testDir(t)
	c, err := Open("marshal-failure")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Append(Message{Role: "user", Content: "existing", TS: time.Now()}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(c.Path())
	if err != nil {
		t.Fatal(err)
	}

	err = c.Append(
		Message{Role: "user", Content: "must not be appended", TS: time.Now()},
		Message{Role: "assistant", Content: "invalid", TS: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
	)
	if err == nil {
		t.Fatal("Append() succeeded with an unmarshalable message")
	}
	after, err := os.ReadFile(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Errorf("file changed after marshal failure\nbefore: %q\nafter:  %q", before, after)
	}
}

func TestAppendMarshalFailureDoesNotCreateFilesystemEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)
	c, err := Open("marshal-failure")
	if err != nil {
		t.Fatal(err)
	}

	err = c.Append(
		Message{Role: "user", Content: "must not be appended", TS: time.Now()},
		Message{Role: "assistant", Content: "invalid", TS: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
	)
	if err == nil {
		t.Fatal("Append() succeeded with an unmarshalable message")
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("failed append created conversation directory: %v", err)
	}
	if _, err := os.Stat(c.Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("failed append created conversation file: %v", err)
	}
}

func TestAppendConcurrentTurnsRemainAdjacent(t *testing.T) {
	testDir(t)
	c, err := Open("concurrent-turns")
	if err != nil {
		t.Fatal(err)
	}

	const turns = 100
	start := make(chan struct{})
	errs := make(chan error, turns)
	var wg sync.WaitGroup
	for i := 0; i < turns; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			content := fmt.Sprintf("turn-%d", i)
			errs <- c.Append(
				Message{Role: "user", Content: content, TS: time.Now()},
				Message{Role: "assistant", Content: content, TS: time.Now()},
			)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	got, skipped, err := c.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(got) != turns*2 {
		t.Fatalf("got %d messages, want %d", len(got), turns*2)
	}
	for i := 0; i < len(got); i += 2 {
		if got[i].Role != "user" || got[i+1].Role != "assistant" || got[i].Content != got[i+1].Content {
			t.Fatalf("messages %d and %d are not an atomic turn: %+v, %+v", i, i+1, got[i], got[i+1])
		}
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
	// Open must not create the directory either.
	c, err := Open("some-name")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open created directory: %v", err)
	}
	// But Append does create both the directory and the file.
	if err := c.Append(Message{Role: "user", Content: "hi", TS: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); err != nil {
		t.Errorf("Append did not create directory: %v", err)
	}
	if _, err := os.Stat(c.Path()); err != nil {
		t.Errorf("Append did not create file: %v", err)
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
	dir := testDir(t)
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
	blockerBefore, err := os.ReadFile(c2.Path())
	if err != nil {
		t.Fatal(err)
	}
	err = Rename("new-name", "blocker")
	if err == nil {
		t.Fatal("renaming onto existing should error")
	}
	const wantPrefix = `rename conversation "new-name" to "blocker": `
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("error = %q, want prefix %q", err, wantPrefix)
	}
	blockerAfter, err := os.ReadFile(c2.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(blockerAfter, blockerBefore) {
		t.Errorf("existing destination changed\nbefore: %q\nafter:  %q", blockerBefore, blockerAfter)
	}
	if _, err := os.Stat(filepath.Join(dir, "new-name.jsonl")); err != nil {
		t.Errorf("source missing after refused rename: %v", err)
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

func TestRemoveOlderThanReportsRemovalFailure(t *testing.T) {
	if os.Getenv(cleanupTargetEnv) != "" {
		target := validateCleanupTarget(t)
		t.Setenv("CLAI_CONVERSATIONS_DIR", target)
		assertRemoveOlderThanReportsRemovalFailure(t)
		return
	}

	dir, err := os.MkdirTemp(os.TempDir(), fixtureDirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
		_ = os.RemoveAll(dir)
	})
	t.Setenv("CLAI_CONVERSATIONS_DIR", dir)

	for _, name := range []string{"undeletable-one", "undeletable-two"} {
		c, err := Open(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Append(Message{Role: "user", Content: "hi", TS: time.Now()}); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-40 * 24 * time.Hour)
		if err := os.Chtimes(c.Path(), old, old); err != nil {
			t.Fatal(err)
		}
	}

	// Write the marker file before locking permissions.
	if err := os.WriteFile(filepath.Join(dir, markerFileName), []byte(markerContent), 0o444); err != nil {
		t.Fatal(err)
	}

	helperPath := ""
	if os.Geteuid() == 0 {
		helperPath = copyTestBinary(t, dir)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	if helperPath != "" {
		runCleanupTestAsUnprivileged(t, helperPath, dir)
		return
	}
	assertRemoveOlderThanReportsRemovalFailure(t)
}

func assertRemoveOlderThanReportsRemovalFailure(t *testing.T) {
	t.Helper()
	removed, err := RemoveOlderThan(30 * 24 * time.Hour)
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if err == nil {
		t.Fatal("RemoveOlderThan() returned nil error after removal failure")
	}
	for _, name := range []string{"undeletable-one.jsonl", "undeletable-two.jsonl"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error = %q, want affected entry %q", err, name)
		}
	}
}

func copyTestBinary(t *testing.T, dir string) string {
	t.Helper()

	src, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	path := filepath.Join(dir, "cleanup-test-helper")
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o555)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCleanupTestAsUnprivileged(t *testing.T, helperPath, dir string) {
	t.Helper()

	// Pass the target directory explicitly; strip any inherited CLAI_CONVERSATIONS_DIR.
	env := []string{cleanupTargetEnv + "=" + dir}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAI_CONVERSATIONS_DIR=") {
			continue
		}
		if strings.HasPrefix(e, cleanupTargetEnv+"=") {
			continue
		}
		env = append(env, e)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helperPath, "-test.run=^"+t.Name()+"$", "-test.count=1")
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid: 65534, Gid: 65534, NoSetGroups: true,
	}}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unprivileged cleanup helper failed: %v\n%s", err, output)
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
