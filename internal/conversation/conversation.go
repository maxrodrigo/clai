package conversation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const fileExt = ".jsonl"

// maxNewRetries caps the number of O_EXCL retry attempts in New() to prevent
// infinite loops if the filesystem consistently races with another process.
const maxNewRetries = 5

// Message is a single turn persisted as one JSON line.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Model     string    `json:"model,omitempty"`
	TS        time.Time `json:"ts"`
	TokensIn  int       `json:"tokens_in,omitempty"`
	TokensOut int       `json:"tokens_out,omitempty"`
}

// Conversation is a handle to a JSONL conversation file.
type Conversation struct {
	Name  string
	path  string
	isNew bool
}

// IsNew reports whether this conversation was just created by New().
func (c *Conversation) IsNew() bool { return c.isNew }

// Path returns the absolute path to the backing JSONL file.
func (c *Conversation) Path() string { return c.path }

// Dir returns the conversations directory path.
// Resolution order:
//  1. $CLAI_CONVERSATIONS_DIR
//  2. $XDG_STATE_HOME/clai/conversations
//  3. ~/.local/state/clai/conversations
func Dir() (string, error) {
	if d := os.Getenv("CLAI_CONVERSATIONS_DIR"); d != "" {
		return d, nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "clai", "conversations"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("conversation dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "clai", "conversations"), nil
}

// ensureDir creates the conversations directory if it does not exist.
// Only called by write paths (Open, New).
func ensureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure conversation dir: %w", err)
	}
	return dir, nil
}

// scanDir reads the conversations directory entries.
// If the directory does not exist, returns empty entries and nil error.
// NEVER creates directories.
func scanDir() (string, []os.DirEntry, error) {
	dir, err := Dir()
	if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return dir, nil, nil
		}
		return "", nil, fmt.Errorf("scan conversation dir: %w", err)
	}
	return dir, entries, nil
}

// Open returns a handle to a named conversation. Neither the file nor the
// directory is created; the first Append creates both if needed.
// IsNew reports whether the file existed at open time.
func Open(name string) (*Conversation, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name+fileExt)
	_, statErr := os.Stat(path)
	return &Conversation{
		Name:  name,
		path:  path,
		isNew: errors.Is(statErr, fs.ErrNotExist),
	}, nil
}

// Append writes a message as a single JSON line to the conversation file.
// Creates the directory and file on first call. Uses O_APPEND with an
// exclusive flock for safe concurrent writes.
func (c *Conversation) Append(m Message) error {
	// Ensure the directory exists on the first write.
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("create conversation dir: %w", err)
	}
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("append conversation %s: %w", c.Name, err)
	}
	defer f.Close()

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("flock conversation %s: %w", c.Name, err)
	}

	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	data = append(data, '\n')

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write conversation %s: %w", c.Name, err)
	}
	return nil
}

// Messages reads all messages from the conversation file.
// Returns parsed messages, a count of malformed lines that were skipped, and any error.
// If the file does not exist, returns empty results with no error.
func (c *Conversation) Messages() ([]Message, int, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("read conversation %s: %w", c.Name, err)
	}

	var msgs []Message
	var skipped int
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(line, "\n")
		if line == "" {
			continue
		}
		var m Message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			skipped++
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, skipped, nil
}

// New creates a new conversation from free-form user input.
// The input is slugified and deduplicated with numeric suffixes if needed.
// If a race on O_EXCL occurs, it rescans and retries up to maxNewRetries times.
func New(input string) (*Conversation, error) {
	base := Slugify(input)
	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < maxNewRetries; attempt++ {
		name, err := nextFreeName(dir, base)
		if err != nil {
			return nil, err
		}

		path := filepath.Join(dir, name+fileExt)
		// O_EXCL guards against races: only one process wins.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue // another process won; rescan and retry
			}
			return nil, fmt.Errorf("create conversation %s: %w", name, err)
		}
		_ = f.Close()

		return &Conversation{
			Name:  name,
			path:  path,
			isNew: true,
		}, nil
	}

	return nil, fmt.Errorf("create conversation: failed after %d attempts (name contention on %q)", maxNewRetries, base)
}

// nextFreeName scans dir for the next available name based on base slug.
// Sequence: base, base-2, base-3, ...
func nextFreeName(dir, base string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	existing := make(map[string]struct{})
	for _, e := range entries {
		n := strings.TrimSuffix(e.Name(), fileExt)
		existing[n] = struct{}{}
	}

	if _, ok := existing[base]; !ok {
		return base, nil
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, ok := existing[candidate]; !ok {
			return candidate, nil
		}
	}
}

// Latest returns the most recently modified conversation.
func Latest() (*Conversation, error) {
	dir, entries, err := scanDir()
	if err != nil {
		return nil, err
	}

	var best os.DirEntry
	var bestTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), fileExt) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == nil || info.ModTime().After(bestTime) {
			best = e
			bestTime = info.ModTime()
		}
	}
	if best == nil {
		return nil, errors.New("no conversations found")
	}
	name := strings.TrimSuffix(best.Name(), fileExt)
	return &Conversation{
		Name: name,
		path: filepath.Join(dir, best.Name()),
	}, nil
}

// Summary holds metadata for listing conversations.
type Summary struct {
	Name    string
	Model   string
	Preview string
	ModTime time.Time
}

// List returns summaries of all conversations, sorted by ModTime descending.
func List() ([]Summary, error) {
	dir, entries, err := scanDir()
	if err != nil {
		return nil, err
	}

	var sums []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), fileExt) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), fileExt)
		c := &Conversation{Name: name, path: filepath.Join(dir, e.Name())}
		msgs, _, err := c.Messages()
		if err != nil {
			continue
		}
		sums = append(sums, Summary{
			Name:    name,
			Model:   LastModel(msgs),
			Preview: firstUserPreview(msgs),
			ModTime: info.ModTime(),
		})
	}

	slices.SortFunc(sums, func(a, b Summary) int {
		return b.ModTime.Compare(a.ModTime) // descending
	})
	return sums, nil
}

// LastSystem returns the last system message from msgs, or nil.
func LastSystem(msgs []Message) *Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "system" {
			return &msgs[i]
		}
	}
	return nil
}

// LastModel returns the model string from the last message that has one set.
func LastModel(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Model != "" {
			return msgs[i].Model
		}
	}
	return ""
}

// firstUserPreview returns a preview (≤48 chars at word boundary) of the
// first user message content.
func firstUserPreview(msgs []Message) string {
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		s := strings.Join(strings.Fields(m.Content), " ") // collapse whitespace
		runes := []rune(s)
		if len(runes) <= 48 {
			return s
		}
		// Truncate at a word boundary, rune-safe for multibyte input.
		s = string(runes[:48])
		if i := strings.LastIndexByte(s, ' '); i > 0 {
			s = s[:i]
		}
		return s
	}
	return ""
}

// Rename renames a conversation file from oldName to newName.
func Rename(oldName, newName string) error {
	if err := ValidateName(oldName); err != nil {
		return err
	}
	if err := ValidateName(newName); err != nil {
		return err
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	oldPath := filepath.Join(dir, oldName+fileExt)
	newPath := filepath.Join(dir, newName+fileExt)

	if _, err := os.Stat(oldPath); err != nil {
		return fmt.Errorf("rename conversation: source %q not found", oldName)
	}
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("rename conversation: target %q already exists", newName)
	}
	return os.Rename(oldPath, newPath)
}

// Remove deletes a conversation file.
func Remove(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name+fileExt)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("remove conversation: %q not found", name)
	}
	return os.Remove(path)
}

// RemoveOlderThan deletes conversations with a modification time older than
// the given age. Returns the number of conversations removed.
func RemoveOlderThan(age time.Duration) (int, error) {
	dir, entries, err := scanDir()
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-age)
	var removed int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), fileExt) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
