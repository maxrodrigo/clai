// Package prompt handles prompt file loading, parsing, and resolution.
package prompt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/maxrodrigo/clai/internal/config"
	"gopkg.in/yaml.v3"
)

// LiteralPath is the sentinel path used for literal/inline prompts.
const LiteralPath = "(literal)"

// IsSystemPath reports whether path is under the system data directory.
// Returns false if no system directory is configured.
func IsSystemPath(path string) bool {
	if systemDir == "" || path == "" {
		return false
	}
	return strings.HasPrefix(path, systemDir)
}

// NotFoundError is returned when a prompt cannot be resolved by any source.
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("unknown prompt: %q — run 'clai prompts' to see available prompts", e.Name)
}

// validSegment matches a single name segment: alphanumeric, hyphens, underscores,
// must not start with a hyphen.
var validSegment = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_-]*$`)

// IsValidName reports whether name is a valid bare prompt name (no namespace).
func IsValidName(name string) bool {
	return validSegment.MatchString(name)
}

// IsNamespaced reports whether name is in owner/prompt format.
func IsNamespaced(name string) bool {
	owner, prompt, found := strings.Cut(name, "/")
	if !found {
		return false
	}
	return validSegment.MatchString(owner) && validSegment.MatchString(prompt)
}

// ParseNamespace splits an owner/name into its two parts.
// Returns owner="", prompt=name for bare names.
func ParseNamespace(name string) (owner, promptName string) {
	owner, promptName, found := strings.Cut(name, "/")
	if !found {
		return "", name
	}
	return owner, promptName
}

// Prompt holds a loaded prompt file with its parsed frontmatter and content.
type Prompt struct {
	Name        string
	Path        string
	Content     string
	Frontmatter Frontmatter
	rawBody     string // original body before prepend/append (for extends)
}

// Frontmatter is the YAML header at the top of a prompt file.
type Frontmatter struct {
	Description string   `yaml:"description,omitempty"` // prompts.chat compatible
	Model       string   `yaml:"model,omitempty"`
	Temperature *float64 `yaml:"temperature,omitempty"` // pointer: nil means not set, allows explicit 0.0
	Strategy    string   `yaml:"strategy,omitempty"`
	Schema      string   `yaml:"schema,omitempty"`
	Think       bool     `yaml:"think,omitempty"`
	Extends     string   `yaml:"extends,omitempty"` // inherit from another named prompt
	Prepend     []string `yaml:"prepend,omitempty"` // files to prepend before prompt body
	Append      []string `yaml:"append,omitempty"`  // files to append after prompt body
}

// Resolve finds a prompt by name, searching registered sources in priority order.
// If the prompt uses extends, it resolves the base and merges them.
func Resolve(name string) (*Prompt, error) {
	return resolve(name, nil)
}

// resolve is the internal implementation with cycle detection.
// chain: names seen so far, for cycle detection (first match wins per registry order).
func resolve(name string, chain []string) (*Prompt, error) {
	var p *Prompt

	for _, src := range registry {
		var err error
		p, err = src.Resolve(name)
		if err != nil {
			return nil, err
		}
		if p != nil {
			break
		}
	}

	if p == nil {
		return nil, &NotFoundError{Name: name}
	}

	if p.Frontmatter.Extends != "" {
		if p.Frontmatter.Extends == p.Name {
			return nil, fmt.Errorf(
				"prompt %q cannot extend itself — use a different name (e.g., 'my-%s' extends '%s')",
				p.Name, p.Name, p.Name,
			)
		}

		if slices.Contains(chain, p.Name) {
			return nil, fmt.Errorf("circular extends: %q", p.Name)
		}

		base, err := resolve(p.Frontmatter.Extends, append(chain, p.Name))
		if err != nil {
			return nil, fmt.Errorf("prompt %q extends %q: %w", p.Name, p.Frontmatter.Extends, err)
		}

		p, err = applyExtends(p, base)
		if err != nil {
			return nil, fmt.Errorf("prompt %q extends %q: %w", name, p.Frontmatter.Extends, err)
		}
	}

	return p, nil
}

// applyExtends merges the base prompt into the child.
// Child fields take precedence; zero values fall back to base.
// Prepend, Append, and Extends are child-only and never inherited.
func applyExtends(child, base *Prompt) (*Prompt, error) {
	merged := base.Frontmatter

	// Child fields override base — only copy non-zero values.
	if child.Frontmatter.Description != "" {
		merged.Description = child.Frontmatter.Description
	}
	if child.Frontmatter.Model != "" {
		merged.Model = child.Frontmatter.Model
	}
	if child.Frontmatter.Temperature != nil {
		merged.Temperature = child.Frontmatter.Temperature
	}
	if child.Frontmatter.Strategy != "" {
		merged.Strategy = child.Frontmatter.Strategy
	}
	if child.Frontmatter.Schema != "" {
		merged.Schema = child.Frontmatter.Schema
	}
	if child.Frontmatter.Think {
		merged.Think = true
	}
	// Prepend/Append/Extends are child-only — never inherited from base.
	merged.Extends = ""
	merged.Prepend = nil
	merged.Append = nil

	body := child.rawBody
	if body == "" {
		body = base.Content
	}

	content, err := assembleContent(child.Path, child.Frontmatter, body)
	if err != nil {
		return nil, err
	}

	return &Prompt{
		Name:        child.Name,
		Path:        child.Path,
		Content:     content,
		Frontmatter: merged,
		rawBody:     child.rawBody,
	}, nil
}

// List returns all available prompts from all sources.
// First-seen wins by name (earlier sources take precedence).
func List() ([]*Prompt, error) {
	seen := map[string]bool{}
	var prompts []*Prompt

	for _, src := range registry {
		srcPrompts, err := src.List()
		if err != nil {
			return nil, err
		}
		for _, p := range srcPrompts {
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			prompts = append(prompts, p)
		}
	}
	return prompts, nil
}

// SourceGroup holds prompts from a single source.
type SourceGroup struct {
	Source  string
	Prompts []*Prompt
}

// ListBySource returns prompts grouped by source, in source priority order.
// Each prompt appears only in its highest-priority source (first-seen wins).
func ListBySource() ([]SourceGroup, error) {
	seen := map[string]bool{}
	var groups []SourceGroup

	for _, src := range registry {
		srcPrompts, err := src.List()
		if err != nil {
			return nil, err
		}

		var groupPrompts []*Prompt
		for _, p := range srcPrompts {
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			groupPrompts = append(groupPrompts, p)
		}

		if len(groupPrompts) > 0 {
			groups = append(groups, SourceGroup{
				Source:  src.Name(),
				Prompts: groupPrompts,
			})
		}
	}
	return groups, nil
}

// Parse parses a prompt file's frontmatter and body.
// Note: extends resolution happens in Resolve, not here — Parse is pure.
func Parse(name, path string, data []byte) (*Prompt, error) {
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parsing prompt %s: %w", path, err)
	}

	rawBody := strings.TrimSpace(body)
	content, err := assembleContent(path, fm, rawBody)
	if err != nil {
		return nil, fmt.Errorf("parsing prompt %s: %w", path, err)
	}

	return &Prompt{
		Name:        name,
		Path:        path,
		Content:     content,
		Frontmatter: fm,
		rawBody:     rawBody,
	}, nil
}

// assembleContent resolves prepend/append files and concatenates them with the prompt body.
// Order: [prepend files...] + body + [append files...]
func assembleContent(promptPath string, fm Frontmatter, body string) (string, error) {
	if len(fm.Prepend) == 0 && len(fm.Append) == 0 {
		return body, nil
	}

	baseDir, err := resolveBaseDir(promptPath)
	if err != nil {
		return "", err
	}

	var parts []string

	for _, ref := range fm.Prepend {
		content, err := readReferencedFile(baseDir, ref)
		if err != nil {
			return "", fmt.Errorf("prepend %q: %w", ref, err)
		}
		parts = append(parts, content)
	}

	parts = append(parts, body)

	for _, ref := range fm.Append {
		content, err := readReferencedFile(baseDir, ref)
		if err != nil {
			return "", fmt.Errorf("append %q: %w", ref, err)
		}
		parts = append(parts, content)
	}

	return strings.Join(parts, "\n\n"), nil
}

// resolveBaseDir determines the directory against which relative paths are resolved.
func resolveBaseDir(promptPath string) (string, error) {
	if promptPath == LiteralPath {
		return os.Getwd()
	}
	return filepath.Dir(promptPath), nil
}

// readReferencedFile reads a file referenced in prepend/append.
// Relative paths are resolved against baseDir.
// Paths starting with ~ are expanded to the user's home directory.
func readReferencedFile(baseDir, ref string) (string, error) {
	resolved, err := expandPath(ref)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(baseDir, resolved)
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		return filepath.Join(home, path[1:]), nil
	}
	return path, nil
}

// splitFrontmatter splits a markdown file into typed frontmatter and body.
// Expects the format: "---\n<yaml>\n---\n<body>".
// Returns an empty Frontmatter if no frontmatter delimiter is present.
// Unknown YAML keys are rejected to catch typos early.
func splitFrontmatter(data []byte) (Frontmatter, string, error) {
	var fm Frontmatter

	// Normalize Windows line endings.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	if !bytes.HasPrefix(data, []byte("---\n")) {
		return fm, string(data), nil
	}

	// data[4:] skips the opening "---\n".
	parts := bytes.SplitN(data[4:], []byte("\n---\n"), 2)
	if len(parts) != 2 {
		// No closing delimiter — treat the whole file as body.
		return fm, string(data), nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(parts[0]))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		return fm, "", fmt.Errorf("invalid frontmatter: %w", err)
	}

	return fm, string(parts[1]), nil
}

// MergeDefaults applies prompt frontmatter defaults (lower priority than
// user/project config and env — only fills in values not already set).
func MergeDefaults(cfg *config.Config, fm Frontmatter) *config.Config {
	out := *cfg

	if fm.Model != "" && out.Model == "" {
		out.Model = fm.Model
	}
	if fm.Temperature != nil && out.Temperature == nil {
		out.Temperature = fm.Temperature
	}
	if fm.Strategy != "" && out.Strategy == "" {
		out.Strategy = fm.Strategy
	}
	if fm.Think && !out.Think {
		out.Think = fm.Think
	}
	return &out
}
