package skillmeta

import "testing"

func TestParseNone(t *testing.T) {
	_, found, problems := Parse([]byte("# Just markdown\n"))
	if found || problems != nil {
		t.Fatalf("found=%v problems=%v, want none", found, problems)
	}
}

func TestParseValid(t *testing.T) {
	fm, found, problems := Parse([]byte("---\nname: demo\ndescription: A demo skill.\n---\n# Body\n"))
	if !found || len(problems) != 0 {
		t.Fatalf("found=%v problems=%v", found, problems)
	}
	if fm.Name != "demo" || fm.Description != "A demo skill." {
		t.Fatalf("parsed %+v", fm)
	}
}

func TestParseUnterminated(t *testing.T) {
	_, found, problems := Parse([]byte("---\nname: demo\n# no close\n"))
	if !found || len(problems) != 1 || problems[0].Code != CodeUnterminated {
		t.Fatalf("found=%v problems=%v", found, problems)
	}
}

func TestParseInvalidYAML(t *testing.T) {
	_, found, problems := Parse([]byte("---\nname: [unclosed\n---\n# Body\n"))
	if !found || len(problems) != 1 || problems[0].Code != CodeInvalidYAML {
		t.Fatalf("found=%v problems=%v", found, problems)
	}
}

func TestValidateHappy(t *testing.T) {
	problems := Validate("demo", Frontmatter{Name: "demo", Description: "ok"})
	if len(problems) != 0 {
		t.Fatalf("problems=%v", problems)
	}
}

func TestValidateMismatchAndDescription(t *testing.T) {
	problems := Validate("manifest-name", Frontmatter{Name: "other", Description: ""})
	if len(problems) != 2 {
		t.Fatalf("problems=%v, want mismatch + missing description", problems)
	}
	if problems[0].Code != CodeNameMismatch || problems[1].Code != CodeMissingDescription {
		t.Fatalf("problem codes: %v", problems)
	}
}

func TestValidateMissingName(t *testing.T) {
	problems := Validate("x", Frontmatter{Description: "d"})
	if len(problems) != 1 || problems[0].Code != CodeMissingName {
		t.Fatalf("problems=%v", problems)
	}
}

func TestCRLFContentIsNotFrontmatter(t *testing.T) {
	// A file starting with "---\r\n" is not a frontmatter block for our
	// purposes; it must not be parsed as one.
	_, found, problems := Parse([]byte("---\r\nname: demo\r\n---\r\n"))
	if found {
		t.Fatalf("found=%v problems=%v, want no frontmatter detected", found, problems)
	}
}
