package localfs

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

const attributesPath = ".gitattributes"

// GeneratedAttributes returns the deterministic Creed-owned attribute block for
// a target. Existing user-authored content is retained byte-for-byte outside
// that target's block. Disabled targets are deliberately not removed: Creed
// never deletes an attribute rule it cannot prove is still exclusively owned.
func (e *Emitter) GeneratedAttributes(_ context.Context, target domain.Target, files []ports.EmittedFile) (*ports.EmittedFile, error) {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if file.Path == attributesPath {
			continue
		}
		path, err := cleanOutputPath(file.Path)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, nil
	}
	sort.Strings(paths)
	paths = compactPaths(paths)

	attributesFile, err := e.safeOutputPath(attributesPath)
	if err != nil {
		return nil, err
	}
	existing, err := os.ReadFile(attributesFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", attributesPath, err)
	}
	content, err := replaceAttributeBlock(string(existing), target.Name, paths)
	if err != nil {
		return nil, err
	}
	return &ports.EmittedFile{Path: attributesPath, Content: []byte(content)}, nil
}

func compactPaths(paths []string) []string {
	if len(paths) < 2 {
		return paths
	}
	out := paths[:1]
	for _, path := range paths[1:] {
		if path != out[len(out)-1] {
			out = append(out, path)
		}
	}
	return out
}

func replaceAttributeBlock(existing, target string, paths []string) (string, error) {
	if strings.ContainsAny(target, "\r\n") || target == "" {
		return "", fmt.Errorf("invalid target name for generated attributes")
	}
	begin := "# creed:generated " + target + " begin"
	end := "# creed:generated " + target + " end"
	blockLines := append([]string{begin}, make([]string, 0, len(paths)+2)...)
	for _, path := range paths {
		blockLines = append(blockLines, path+" linguist-generated=true")
	}
	blockLines = append(blockLines, end)
	if strings.Contains(existing, strings.Join(blockLines, "\n")) {
		return existing, nil
	}
	lines := strings.Split(existing, "\n")
	out := make([]string, 0, len(lines)+len(paths)+3)
	inside := false
	found := false
	for _, line := range lines {
		switch line {
		case begin:
			if inside || found {
				return "", fmt.Errorf("duplicate or nested Creed attribute block for target %q", target)
			}
			inside, found = true, true
			continue
		case end:
			if !inside {
				return "", fmt.Errorf("unmatched Creed attribute block end for target %q", target)
			}
			inside = false
			continue
		}
		if !inside {
			out = append(out, line)
		}
	}
	if inside {
		return "", fmt.Errorf("unterminated Creed attribute block for target %q", target)
	}
	// Preserve user-authored bytes outside Creed blocks, including deliberate
	// trailing blank lines. Add only the separator needed when the existing
	// content has no terminal newline.
	prefix := strings.Join(out, "\n")
	if prefix != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	block := append([]string{begin}, make([]string, 0, len(paths)+2)...)
	for _, path := range paths {
		block = append(block, path+" linguist-generated=true")
	}
	block = append(block, end, "")
	return prefix + strings.Join(block, "\n"), nil
}
