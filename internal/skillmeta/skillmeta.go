// Package skillmeta implements creed's skill discovery contract: the YAML
// frontmatter a SKILL.md (or flat skill file) may carry, and the rules that
// frontmatter must satisfy. It is shared by validation (diagnostics) and
// sync rendering (generation-time enforcement) so the two cannot drift.
package skillmeta

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds the skill discovery fields creed interprets.
// All other frontmatter keys pass through untouched.
type Frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Problem describes one skill-contract violation.
type Problem struct {
	// Code is a stable machine-readable identifier.
	Code string
	// Message names the violation for humans.
	Message string
}

const (
	// CodeUnterminated means a frontmatter block opened but never closed.
	CodeUnterminated = "unterminated_skill_frontmatter"
	// CodeInvalidYAML means the frontmatter block is not valid YAML.
	CodeInvalidYAML = "invalid_skill_frontmatter"
	// CodeMissingName means frontmatter exists but carries no name.
	CodeMissingName = "missing_skill_name"
	// CodeNameMismatch means the frontmatter name differs from the manifest name.
	CodeNameMismatch = "skill_name_mismatch"
	// CodeMissingDescription means the frontmatter carries no description.
	CodeMissingDescription = "missing_skill_description"
)

// Parse extracts frontmatter from skill content. found is false when the
// content carries no frontmatter block at all (legal, but callers should
// warn: downstream tools discover skills through these fields). A block
// that opens but never closes yields a CodeUnterminated problem.
func Parse(content []byte) (fm Frontmatter, found bool, problems []Problem) {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return Frontmatter{}, false, nil
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Frontmatter{}, true, []Problem{{
			Code:    CodeUnterminated,
			Message: "frontmatter block is opened but never closed",
		}}
	}
	block := rest[:end]
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return Frontmatter{}, true, []Problem{{
			Code:    CodeInvalidYAML,
			Message: fmt.Sprintf("frontmatter is not valid YAML: %v", err),
		}}
	}
	return fm, true, nil
}

// Validate checks parsed frontmatter against the manifest-declared skill
// name. It requires a present name that matches, and a non-empty
// description. Problems reference the manifest name so callers can prefix
// the file path when surfacing them.
func Validate(manifestName string, fm Frontmatter) []Problem {
	var problems []Problem
	if strings.TrimSpace(fm.Name) == "" {
		problems = append(problems, Problem{
			Code:    CodeMissingName,
			Message: "frontmatter has no name",
		})
		return problems
	}
	if fm.Name != manifestName {
		problems = append(problems, Problem{
			Code:    CodeNameMismatch,
			Message: fmt.Sprintf("frontmatter name %q does not match manifest name %q", fm.Name, manifestName),
		})
	}
	if strings.TrimSpace(fm.Description) == "" {
		problems = append(problems, Problem{
			Code:    CodeMissingDescription,
			Message: "frontmatter has no description",
		})
	}
	return problems
}
