package usecase

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnifiedFileDiffIsApplicableForSeparatedEditsAndEOFNewline(t *testing.T) {
	separated := unifiedFileDiff("example.txt", []byte("one\ntwo\nthree\n"), []byte("ONE\ntwo\nTHREE\n"), true, true)
	if !strings.Contains(separated, " two\n") || !strings.Contains(separated, "@@ -1,3 +1,3 @@") {
		t.Fatalf("separated diff lacks required context or ranges: %q", separated)
	}
	noNewline := unifiedFileDiff("example.txt", []byte("one\n"), []byte("one"), true, true)
	if !strings.Contains(noNewline, "\\ No newline at end of file\n") || !strings.Contains(noNewline, "-one\n") || !strings.Contains(noNewline, "+one\n") {
		t.Fatalf("EOF newline change is not represented: %q", noNewline)
	}
}

func TestEmptyFileDiffIsApplicable(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		setup   func(t *testing.T, root string)
	}{
		{name: "create", content: emptyFileDiff("empty.txt", false)},
		{name: "delete", content: emptyFileDiff("empty.txt", true), setup: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "empty.txt"), nil, 0644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.setup != nil {
				test.setup(t, root)
			}
			patch := filepath.Join(root, "change.patch")
			if err := os.WriteFile(patch, []byte(test.content), 0644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("patch", "--dry-run", "-p1", "-i", patch)
			cmd.Dir = root
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("patch rejected empty-file diff: %v\n%s", err, output)
			}
		})
	}
}
