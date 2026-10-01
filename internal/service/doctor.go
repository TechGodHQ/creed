package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/techgodhq/creed/internal/usecase"
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
	Drifted      bool                  `json:"drifted"`
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
	requiresGit := false
	if manifest, err := s.readManifest(); err == nil {
		report.SourceType = manifest.Source.Type
		requiresGit = manifest.Source.Type == "git"
		remotes := []string{}
		if manifest.Source.Remote != "" {
			remotes = append(remotes, redactRemoteURL(manifest.Source.Remote))
		}
		for _, layer := range manifest.Source.Layers {
			requiresGit = requiresGit || layer.Type == "git"
			if layer.Remote != "" {
				remotes = append(remotes, redactRemoteURL(layer.Remote))
			}
		}
		report.SourceRemote = strings.Join(remotes, ", ")
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

	// --- Generated-output drift ---
	// A healthy manifest alone says nothing about whether its declared outputs
	// are current. Reuse the canonical diff engine so doctor reports exactly
	// the same target drift that CI can gate with `creed diff`.
	if validation.Valid {
		diff, err := s.Diff(ctx, usecase.DiffOptions{})
		if err != nil {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "error",
				Code:    "output_drift_check_failed",
				Message: "generated output drift could not be checked",
				Detail:  err.Error(),
			})
		} else if diff.HasDifferences() {
			report.Drifted = true
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "error",
				Code:    "output_drift",
				Message: "generated target output differs from the current Creed source",
				Detail:  "Run 'creed diff' to inspect changes, then 'creed sync' to update generated output.",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Kind:    "info",
				Code:    "output_drift",
				Message: "generated target output is current",
			})
		}
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
		if requiresGit {
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
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	parsed, err := url.Parse(remote)
	if err == nil && parsed.Scheme != "" {
		// Drop all userinfo. Usernames can themselves be bearer tokens.
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	// scp-style SSH remotes have no URL scheme. If parsing failed but the
	// value still contains userinfo, strip everything through the last @.
	if at := strings.LastIndex(remote, "@"); at >= 0 {
		if colon := strings.Index(remote, ":"); colon > 0 && colon < at {
			return remote[at+1:]
		}
	}
	return remote
}

func normalizePullRemoteURL(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return remote, nil
	}
	parsed, err := url.Parse(remote)
	if err != nil {
		return "", fmt.Errorf("invalid remote URL: %w", err)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("remote URL must not contain embedded credentials; configure HTTPS/SSH authentication separately")
	}
	if parsed.Scheme == "" {
		// scp-style SSH remotes (git@host:path) are intentionally accepted.
		return remote, nil
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("remote URL must not contain a query or fragment")
	}
	return parsed.String(), nil
}
