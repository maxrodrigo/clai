package conversation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var nameRe = regexp.MustCompile(`^[a-z0-9_][a-z0-9._-]*$`)

// ValidateName checks that name is a valid conversation identifier.
// Rules: lowercase alphanumeric, dots, dashes, underscores; must start with
// alphanumeric or underscore; max 64 characters.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("conversation name must not be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("conversation name too long (%d chars, max 64)", len(name))
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid conversation name %q: must match %s", name, nameRe.String())
	}
	return nil
}

// Slugify derives a short, filesystem-safe slug from free-form user input.
// The result is at most 24 characters, broken at a word boundary, containing
// only [a-z0-9-]. Returns "conversation" if no usable characters remain.
func Slugify(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 {
				last := b.String()
				if last[len(last)-1] != '-' {
					b.WriteByte('-')
				}
			}
		}
	}
	slug := strings.TrimRight(b.String(), "-")
	if slug == "" {
		return "conversation"
	}
	if len(slug) <= 24 {
		return slug
	}
	// Truncate at word (dash) boundary.
	slug = slug[:24]
	if i := strings.LastIndex(slug, "-"); i > 0 {
		slug = slug[:i]
	}
	return slug
}
