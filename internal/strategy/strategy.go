// Package strategy handles reasoning strategy loading and application.
package strategy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// NotFoundError is returned when a strategy cannot be resolved.
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("unknown strategy: %q — run 'clai strategies' to see available strategies", e.Name)
}

// Package-level state set by Init.
var (
	projectDir string
	userDir    string
	systemDir  string
	warn       io.Writer
)

// Init sets the lookup directories and warning writer for strategy resolution.
// Call once from main after determining the config directory.
// w receives warnings about unreadable or malformed strategy files; pass
// os.Stderr to surface them to the user.
//
// dataDir is the system data directory (from datadir.Dir()).
// If empty, system strategies are not available — users must set CLAI_DATA_DIR
// or use a proper install method.
//
// If Init is not called, Resolve and List still work but only return
// project-local and user strategies; system strategies are not searched.
func Init(dataDir, configDir string, w io.Writer) {
	projectDir = filepath.Join(".clai", "strategies")
	userDir = filepath.Join(configDir, "strategies")
	if dataDir != "" {
		systemDir = filepath.Join(dataDir, "strategies")
	}
	warn = w
}

// IsSystemPath reports whether path is under the system data directory.
// Returns false if no system directory is configured.
func IsSystemPath(path string) bool {
	if systemDir == "" || path == "" {
		return false
	}
	return strings.HasPrefix(path, systemDir)
}

// Strategy holds a loaded reasoning strategy.
type Strategy struct {
	Name        string
	Path        string
	Description string
	Prompt      string
}

// Apply prepends the strategy instruction to the system prompt.
func (s *Strategy) Apply(systemPrompt string) string {
	if s == nil {
		return systemPrompt
	}
	return s.Prompt + "\n\n" + systemPrompt
}

// Resolve finds a strategy by name. Search order:
// 1. .clai/strategies/<name>.md (project-local)
// 2. ~/.config/clai/strategies/<name>.md (user)
// 3. <dataDir>/strategies/<name>.md (system)
//
// If Init was not called, only project-local strategies are searched.
func Resolve(name string) (*Strategy, error) {
	if name == "" || name == "none" {
		return nil, nil
	}

	// Project-local
	if projectDir != "" {
		if s, err := loadFromDir(projectDir, name); s != nil || err != nil {
			return s, err
		}
	}
	// User
	if userDir != "" {
		if s, err := loadFromDir(userDir, name); s != nil || err != nil {
			return s, err
		}
	}
	// System
	if systemDir != "" {
		if s, err := loadFromDir(systemDir, name); s != nil || err != nil {
			return s, err
		}
	}

	return nil, &NotFoundError{Name: name}
}

// List returns all available strategies (deduped, project > user > system).
// If Init was not called, only project-local strategies are returned.
func List() ([]*Strategy, error) {
	seen := map[string]bool{}
	var strategies []*Strategy

	dirs := []string{projectDir, userDir, systemDir}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		items, err := listFromDir(dir)
		if err != nil {
			return nil, err
		}
		for _, s := range items {
			if !seen[s.Name] {
				seen[s.Name] = true
				strategies = append(strategies, s)
			}
		}
	}

	return strategies, nil
}

// parse parses a strategy markdown file.
// Strategy files have no frontmatter — they start with an optional "# Title"
// line followed by the prompt content.
func parse(name, path string, data []byte) *Strategy {
	content := strings.TrimSpace(string(data))

	// First line that starts with "# " is the title; skip it for the prompt.
	lines := strings.SplitN(content, "\n", 2)
	strategyPrompt := content
	desc := name
	if len(lines) > 0 && strings.HasPrefix(lines[0], "# ") {
		desc = strings.TrimPrefix(lines[0], "# ")
		if len(lines) > 1 {
			strategyPrompt = strings.TrimSpace(lines[1])
		} else {
			strategyPrompt = ""
		}
	}

	return &Strategy{
		Name:        name,
		Path:        path,
		Description: desc,
		Prompt:      strategyPrompt,
	}
}

// loadFromDir attempts to load a strategy from a directory.
// Returns nil, nil if the strategy is not found.
func loadFromDir(dir, name string) (*Strategy, error) {
	path := filepath.Join(dir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parse(name, path, data), nil
}

// listFromDir lists all strategies in a directory.
// Returns nil, nil if the directory does not exist.
func listFromDir(dir string) ([]*Strategy, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var strategies []*Strategy
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			if warn != nil {
				fmt.Fprintf(warn, "warning: skipping strategy %s: %v\n", path, err)
			}
			continue
		}
		strategies = append(strategies, parse(name, path, data))
	}
	return strategies, nil
}
