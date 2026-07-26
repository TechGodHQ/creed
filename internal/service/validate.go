package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/techgodhq/creed/internal/domain"
	"gopkg.in/yaml.v3"
)

// ValidationDiagnostic identifies one manifest or source-health finding.
type ValidationDiagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// ValidationResult is the structured result of a non-mutating Creed health check.
type ValidationResult struct {
	Valid    bool                   `json:"valid"`
	Errors   []ValidationDiagnostic `json:"errors"`
	Warnings []ValidationDiagnostic `json:"warnings"`
}

// validationManifest is deliberately separate from the permissive manifest
// reader used by sync. Validate must report unknown fields and a missing
// version instead of accepting historical defaults silently.
type validationManifest struct {
	Version *int                 `yaml:"version"`
	Source  validationSource     `yaml:"source"`
	Targets []validationTarget   `yaml:"targets"`
	Skills  []domain.SkillEntry  `yaml:"skills"`
	Configs []domain.ConfigEntry `yaml:"config"`
}

type validationSource struct {
	Type   string `yaml:"type"`
	Path   string `yaml:"path"`
	Remote string `yaml:"remote"`
}

type validationTarget struct {
	Name      string `yaml:"name"`
	Enabled   bool   `yaml:"enabled"`
	OutputDir string `yaml:"output_dir"`
}

// Validate checks the local manifest, enabled targets, and every registered
// source without writing target outputs. Errors are reported in the result;
// a returned error is reserved for an unreadable or unparsable manifest.
func (s *Implementation) Validate(ctx context.Context) (ValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return ValidationResult{}, err
	}
	manifest, err := s.readValidationManifest()
	if err != nil {
		result := ValidationResult{}
		result.addError("invalid_manifest", "manifest is unreadable or does not match the supported schema: "+err.Error(), "manifest.yaml")
		return result, nil
	}
	result := ValidationResult{}
	if manifest.Version == nil {
		result.addError("missing_manifest_version", "manifest version is required", "manifest.yaml")
	} else if *manifest.Version != 1 {
		result.addError("unsupported_manifest_version", fmt.Sprintf("manifest version %d is unsupported", *manifest.Version), "manifest.yaml")
	}
	if manifest.Source.Type != "local" && manifest.Source.Type != "git" {
		result.addError("unknown_source_type", fmt.Sprintf("source type %q is unsupported", manifest.Source.Type), "manifest.yaml")
	}
	if manifest.Source.Type == "git" && strings.TrimSpace(manifest.Source.Remote) == "" {
		result.addError("missing_source_remote", "git source requires a remote URL", "manifest.yaml")
	}

	seenTargets := make(map[string]struct{}, len(manifest.Targets))
	for _, target := range manifest.Targets {
		if target.Name == "" {
			result.addError("empty_target_name", "target name is required", "manifest.yaml")
			continue
		}
		if _, ok := seenTargets[target.Name]; ok {
			result.addError("duplicate_target_name", fmt.Sprintf("target %q is declared more than once", target.Name), "manifest.yaml")
		} else {
			seenTargets[target.Name] = struct{}{}
		}
		known, lookupErr := domain.LookupTarget(target.Name)
		if lookupErr != nil {
			result.addError("unknown_target", lookupErr.Error(), "manifest.yaml")
			continue
		}
		if target.Enabled && len(known.Outputs("")) == 0 {
			result.addWarning("enabled_target_has_no_outputs", fmt.Sprintf("enabled target %q has no configured outputs", target.Name), "manifest.yaml")
		}
	}

	seenNames := map[string]string{}
	seenPaths := map[string]string{}
	for _, entry := range manifest.Skills {
		s.validateEntry(&result, "skill", entry.Name, entry.Path, seenNames, seenPaths)
	}
	for _, entry := range manifest.Configs {
		s.validateEntry(&result, "config", entry.Name, entry.Path, seenNames, seenPaths)
	}
	result.Valid = len(result.Errors) == 0
	return result, nil
}

func (s *Implementation) readValidationManifest() (*validationManifest, error) {
	data, err := os.ReadFile(s.manifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("manifest.yaml does not exist")
		}
		return nil, fmt.Errorf("manifest.yaml cannot be read")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var manifest validationManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("manifest contains more than one YAML document")
	} else if err != io.EOF {
		return nil, err
	}
	return &manifest, nil
}

func (s *Implementation) validateEntry(result *ValidationResult, kind, name, sourcePath string, seenNames, seenPaths map[string]string) {
	label := kind + " " + fmt.Sprintf("%q", name)
	if strings.TrimSpace(name) == "" {
		result.addError("empty_source_name", kind+" name is required", sourcePath)
	} else if previous, ok := seenNames[name]; ok {
		result.addError("duplicate_source_name", fmt.Sprintf("%s duplicates %s", label, previous), sourcePath)
	} else {
		seenNames[name] = label
	}
	cleanPath, err := safeSourcePath(sourcePath)
	if err != nil {
		result.addError("unsafe_source_path", fmt.Sprintf("%s path %q is invalid: %v", label, sourcePath, err), sourcePath)
		return
	}
	if previous, ok := seenPaths[cleanPath]; ok {
		result.addError("duplicate_source_path", fmt.Sprintf("%s reuses source path %q already used by %s", label, cleanPath, previous), cleanPath)
	} else {
		seenPaths[cleanPath] = label
	}

	path := filepath.Join(s.creedDir(), cleanPath)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			result.addError("missing_source_file", fmt.Sprintf("%s source file does not exist", label), cleanPath)
		} else {
			result.addError("unreadable_source_file", fmt.Sprintf("%s source file cannot be inspected", label), cleanPath)
		}
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		result.addError("symlink_source_file", fmt.Sprintf("%s source must be a regular file, not a symlink", label), cleanPath)
		return
	}
	resolvedRoot, rootErr := filepath.EvalSymlinks(s.creedDir())
	resolvedPath, pathErr := filepath.EvalSymlinks(path)
	if rootErr != nil || pathErr != nil {
		result.addError("unreadable_source_file", fmt.Sprintf("%s source path cannot be resolved", label), cleanPath)
		return
	}
	if relative, relErr := filepath.Rel(resolvedRoot, resolvedPath); relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		result.addError("escaped_source_path", fmt.Sprintf("%s resolves outside .creed", label), cleanPath)
		return
	}
	if !info.Mode().IsRegular() {
		result.addError("non_regular_source_file", fmt.Sprintf("%s source must be a regular file", label), cleanPath)
		return
	}
	if info.Mode().Perm()&0o444 == 0 {
		result.addError("unreadable_source_file", fmt.Sprintf("%s source file has no read permission", label), cleanPath)
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		result.addError("unreadable_source_file", fmt.Sprintf("%s source file cannot be read", label), cleanPath)
		return
	}
	if strings.TrimSpace(string(content)) == "" {
		result.addWarning("empty_source_content", fmt.Sprintf("%s source file is empty", label), cleanPath)
	}
}

func safeSourcePath(sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(sourcePath) || filepath.VolumeName(sourcePath) != "" {
		return "", fmt.Errorf("path must be relative to .creed")
	}
	clean := filepath.Clean(sourcePath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must remain inside .creed")
	}
	return clean, nil
}

func (r *ValidationResult) addError(code, message, path string) {
	r.Errors = append(r.Errors, ValidationDiagnostic{Severity: "error", Code: code, Message: message, Path: path})
}

func (r *ValidationResult) addWarning(code, message, path string) {
	r.Warnings = append(r.Warnings, ValidationDiagnostic{Severity: "warning", Code: code, Message: message, Path: path})
}
