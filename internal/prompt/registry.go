package prompt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// source is the interface for prompt sources.
// Implementations can provide prompts from different backends:
// - filesystem (system data dir, user directories)
// - git repositories (future)
// - prompts.chat API (future)
type source interface {
	// Name returns the source identifier (e.g., "system", "user", "prompts.chat").
	Name() string

	// Resolve returns a prompt by name, or nil if not found.
	Resolve(name string) (*Prompt, error)

	// List returns all available prompts from this source.
	List() ([]*Prompt, error)
}

// registry holds all registered sources in priority order.
var registry []source

// systemDir holds the resolved system data directory for IsSystemPath checks.
var systemDir string

// registerSource adds a source to the registry.
// Sources are searched in registration order.
func registerSource(s source) {
	registry = append(registry, s)
}

// resetSources clears all registered sources (for testing).
func resetSources() {
	registry = nil
	systemDir = ""
}

// filesystemSource provides prompts from a directory on disk.
type filesystemSource struct {
	name string
	dir  string
	warn io.Writer
}

// newFilesystemSource creates a source that reads from a directory.
func newFilesystemSource(name, dir string, warn io.Writer) source {
	return &filesystemSource{name: name, dir: dir, warn: warn}
}

func (s *filesystemSource) Name() string { return s.name }

func (s *filesystemSource) Resolve(name string) (*Prompt, error) {
	if IsNamespaced(name) {
		return nil, nil // namespaced names belong to communitySource
	}
	path := filepath.Join(s.dir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // not found, not an error
		}
		return nil, err
	}
	return Parse(name, path, data)
}

func (s *filesystemSource) List() ([]*Prompt, error) {
	return listMarkdownPrompts(s.dir, func(base string) string { return base }, s.warn)
}

// listMarkdownPrompts parses every *.md file in dir into a Prompt. nameFor maps
// a file's base name (without ".md") to the prompt's addressable name.
// Unreadable or malformed files are skipped with a warning. A missing
// directory is not an error.
func listMarkdownPrompts(dir string, nameFor func(base string) string, warn io.Writer) ([]*Prompt, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var prompts []*Prompt
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".md")
		path := filepath.Join(dir, e.Name())
		if p := loadPrompt(nameFor(base), path, warn); p != nil {
			prompts = append(prompts, p)
		}
	}
	return prompts, nil
}

// loadPrompt reads and parses a single prompt file, returning nil and emitting
// a warning if it cannot be read or parsed.
func loadPrompt(name, path string, warn io.Writer) *Prompt {
	warnSkip := func(err error) {
		if warn != nil {
			fmt.Fprintf(warn, "warning: skipping %s: %v\n", path, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		warnSkip(err)
		return nil
	}
	p, err := Parse(name, path, data)
	if err != nil {
		warnSkip(err)
		return nil
	}
	return p
}

// communitySource provides prompts installed from community namespaces.
// Prompts are stored under {dir}/{owner}/{name}.md and addressed as "owner/name".
type communitySource struct {
	dir  string
	warn io.Writer
}

// newCommunitySource creates a source that reads namespaced community prompts
// from subdirectories of dir. Each subdirectory is an owner namespace.
func newCommunitySource(dir string, warn io.Writer) source {
	return &communitySource{dir: dir, warn: warn}
}

func (s *communitySource) Name() string { return "community" }

func (s *communitySource) Resolve(name string) (*Prompt, error) {
	if !IsNamespaced(name) {
		return nil, nil // community source only handles owner/name
	}
	owner, promptName := ParseNamespace(name)
	path := filepath.Join(s.dir, owner, promptName+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // not found, not an error
		}
		return nil, err
	}
	return Parse(name, path, data)
}

func (s *communitySource) List() ([]*Prompt, error) {
	owners, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // directory doesn't exist yet, not an error
		}
		return nil, err
	}
	var prompts []*Prompt
	for _, ownerEntry := range owners {
		if !ownerEntry.IsDir() {
			continue
		}
		owner := ownerEntry.Name()
		ownerDir := filepath.Join(s.dir, owner)
		owned, err := listMarkdownPrompts(ownerDir, func(base string) string {
			return owner + "/" + base
		}, s.warn)
		if err != nil {
			if s.warn != nil {
				fmt.Fprintf(s.warn, "warning: skipping community/%s: %v\n", owner, err)
			}
			continue
		}
		prompts = append(prompts, owned...)
	}
	return prompts, nil
}

// RegisterDefaultSources sets up the standard source chain.
// Call this from main after setting Stderr.
//
// dataDir is the system data directory (from datadir.Dir()).
// If empty, system prompts are not available — users must set CLAI_DATA_DIR
// or use a proper install method.
func RegisterDefaultSources(dataDir, configDir string, warn io.Writer) {
	systemDir = dataDir

	// Order matters: first match wins.
	// 1. Project-local
	registerSource(newFilesystemSource("project", filepath.Join(".clai", "prompts"), warn))
	// 2. User config
	registerSource(newFilesystemSource("user", filepath.Join(configDir, "prompts"), warn))
	// 3. Community (installed via 'clai prompt install')
	registerSource(newCommunitySource(filepath.Join(configDir, "community"), warn))
	// 4. System (from data directory)
	if dataDir != "" {
		registerSource(newFilesystemSource("system", filepath.Join(dataDir, "prompts"), warn))
	}
}
