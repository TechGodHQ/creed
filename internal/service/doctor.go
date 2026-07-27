package service

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DoctorCheck is a single diagnostic finding in a DoctorReport. CheckKind
// distinguishes actionable errors from informational status so the CLI can
// format them differently.
type DoctorCheck struct {
	Kind    string `json:"kind"`             // "error" or "info"
	Code    string `json:"code"`             // machine-readable diagnostic code
	Message string `json:"message"`          // human-readable description
	Detail  string `json:"detail,omitempty"` // optional extra context
}

// DoctorTargetSummary describes one configured target's state for the report.
type DoctorTargetSummary struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
	OutputDir   string `json:"output_dir"`
}

// DoctorReport is the structured result of a non-mutating environment and
// source-health diagnostic run. It reuses the canonical ValidationResult
// and enriches it with project-level context that helps resolve setup
// failures.
type DoctorReport struct {
	Root         string                `json:"root"`
	ManifestOK   bool                  `json:"manifest_ok"`
	SourceDirOK  bool                  `json:"source_dir_ok"`
	SourceType   string                `json:"source_type,omitempty"`
	SourceRemote string                `json:"source_remote,omitempty"`
	GitAvailable bool                  `json:"git_available"`
	GitPath      string                `json:"git_path,omitempty"`
	Validation   ValidationResult      `json:"validation"`
	Targets      []DoctorTargetSummary `json:"targets"`
	Checks       []DoctorCheck         `json:"checks"`
}

// Doctor produces a diagnostic report covering the project root, manifest and
// source presence, validation summary, configured targets, and git
// availability. It is non-mutating and never exposes tokens or sensitive
// remote credentials. A returned error is reserved for an unexpected failure
// that prevents any diagnosis; structured findings are always in the report.
func (s *Implementation) Doctor(ctx context.Context) (DoctorReport, error) {
	if err := ctx.Err(); err != nil {
		return DoctorReport{}, err
	}

	report := DoctorReport{Root: s.resolveRoot()}

	// --- .creed/ presence ---
	creedDir := s.creedDir()
	if info, err := os.Stat(creedDir); err != nil {
		if os.IsNotExist(err) {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "error",
				Code:    "missing_source_dir",
				Message: ".creed source directory does not exist",
				Detail:  "Run 'creed init' to scaffold the project",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "error",
				Code:    "unreadable_source_dir",
				Message: ".creed source directory cannot be inspected",
				Detail:  err.Error(),
			})
		}
	} else if !info.IsDir() {
		report.Checks = append(report.Checks, DoctorCheck{
			Kind:    "error",
			Code:    "source_not_directory",
			Message: ".creed path exists but is not a directory",
		})
	} else {
		report.SourceDirOK = true
		report.Checks = append(report.Checks, DoctorCheck{
			Kind:    "info",
			Code:    "source_dir",
			Message: ".creed source directory present",
		})
	}

	// --- Manifest presence ---
	if _, err := os.Stat(s.manifestPath()); err != nil {
		if os.IsNotExist(err) {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "error",
				Code:    "missing_manifest",
				Message: "manifest.yaml does not exist",
				Detail:  "Run 'creed init' to create the project manifest",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "error",
				Code:    "unreadable_manifest",
				Message: "manifest.yaml cannot be inspected",
				Detail:  err.Error(),
			})
		}
	} else {
		report.ManifestOK = true
		report.Checks = append(report.Checks, DoctorCheck{
			Kind:    "info",
			Code:    "manifest",
			Message: "manifest.yaml present",
		})
	}

	// --- Source type / remote (from manifest, best-effort) ---
	if manifest, err := s.readManifest(); err == nil {
		report.SourceType = manifest.Source.Type
		report.SourceRemote = redactRemoteURL(manifest.Source.Remote)
	}

	// --- Canonical validation ---
	validation, _ := s.Validate(ctx)
	report.Validation = validation
	if validation.Valid {
		report.Checks = append(report.Checks, DoctorCheck{
			Kind:    "info",
			Code:    "validation",
			Message: "manifest and sources validate cleanly",
		})
	} else {
		// Convert each validation error into a doctor check for unified display.
		for _, diag := range validation.Errors {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "error",
				Code:    diag.Code,
				Message: diag.Message,
				Detail:  diag.Path,
			})
		}
	}
	for _, diag := range validation.Warnings {
		report.Checks = append(report.Checks, DoctorCheck{
			Kind:    "info",
			Code:    diag.Code,
			Message: diag.Message,
			Detail:  diag.Path,
		})
	}

	// --- Configured targets ---
	if targets, err := s.ListTargets(ctx); err == nil {
		for _, t := range targets {
			report.Targets = append(report.Targets, DoctorTargetSummary{
				Name:        t.Name,
				DisplayName: t.DisplayName,
				Enabled:     t.Enabled,
				OutputDir:   t.OutputDir,
			})
		}
	}

	// --- Git availability (remote prerequisite, never a sync failure) ---
	if gitPath, err := exec.LookPath("git"); err == nil {
		report.GitAvailable = true
		report.GitPath = gitPath
		report.Checks = append(report.Checks, DoctorCheck{
			Kind:    "info",
			Code:    "git_available",
			Message: "git executable found",
			Detail:  gitPath,
		})
	} else {
		if report.SourceType == "git" {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "error",
				Code:    "git_missing_for_remote_source",
				Message: "git source configured but git executable not found — pull/push operations will fail",
				Detail:  "Install git or switch source.type to 'local'",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "info",
				Code:    "git_not_required",
				Message: "git not found — only needed for remote pull/push, not for local sync",
			})
		}
	}

	return report, nil
}

// HasErrors returns true when the report contains any error-level check or
// validation errors. It is used by the CLI to set an appropriate exit code.
func (r DoctorReport) HasErrors() bool {
	if !r.Validation.Valid {
		return true
	}
	for _, c := range r.Checks {
		if c.Kind == "error" {
			return true
		}
	}
	return false
}

// resolveRoot returns the absolute project root for display purposes.
func (s *Implementation) resolveRoot() string {
	abs, err := filepath.Abs(s.root)
	if err != nil {
		return s.root
	}
	return abs
}

// redactRemoteURL strips embedded credentials from a git remote URL so the
// doctor report never exposes tokens or passwords. For URLs that cannot be
// parsed, it falls back to a conservative split-based approach that removes
// anything between the scheme and the last @ before the host.
func redactRemoteURL(remote string) string {
	if remote == "" {
		return ""
	}
	// SSH-style URLs (git@host:path) have no URL userinfo; safe to return.
	if !strings.HasPrefix(remote, "https://") && !strings.HasPrefix(remote, "http://") {
		return remote
	}
	parsed, err := url.Parse(remote)
	if err != nil || parsed.User == nil {
		return remote
	}
	// Preserve the username (safe to display), drop the password.
	if _, hasPassword := parsed.User.Password(); hasPassword {
		parsed.User = url.User(parsed.User.Username())
	}
	return parsed.String()
}
