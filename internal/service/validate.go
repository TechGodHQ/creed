package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/techgodhq/creed/internal/adapters/localfs"
	"github.com/techgodhq/creed/internal/domain"
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
	Type     string            `yaml:"type"`
	Path     string            `yaml:"path"`
	Remote   string            `yaml:"remote"`
	Ref      string            `yaml:"ref"`
	Layers   []validationLayer `yaml:"layers"`
	Overlays []validationLayer `yaml:"overlays"`
}

type validationLayer struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	Path   string `yaml:"path"`
	Remote string `yaml:"remote"`
	Ref    string `yaml:"ref"`
}

type validationTarget struct {
	Name      string `yaml:"name"`
	Enabled   bool   `yaml:"enabled"`
	OutputDir string `yaml:"output_dir"`
}

// Validate checks the local manifest, all configured source layers, enabled
// targets, and every registered source file without writing target outputs.
// Errors are reported in the result; a returned error is reserved for an
// unreadable or unparsable manifest.
func (s *Implementation) Validate(ctx context.Context) (ValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return ValidationResult{}, err
	}
	manifest, err := s.readValidationManifest(ctx)
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

	sourceType := manifest.Source.Type
	if sourceType == "" {
		sourceType = "local"
	}
	if manifest.Source.Type != "local" && manifest.Source.Type != "git" && manifest.Source.Type != "layered" {
		result.addError("unknown_source_type", fmt.Sprintf("source type %q is unsupported", manifest.Source.Type), "manifest.yaml")
	}
	if manifest.Source.Path != "" {
		if _, err := safeSourcePath(manifest.Source.Path); err != nil {
			result.addError("unsafe_source_path", fmt.Sprintf("source path %q is invalid: %v", manifest.Source.Path, err), "source.path")
		}
	}
	if (sourceType == "local" || sourceType == "layered") && manifest.Source.Path != "" && manifest.Source.Path != ".creed" {
		result.addError("unsupported_local_source_path", "local and layered consumer sources must use .creed", "source.path")
	}
	if sourceType == "git" && strings.TrimSpace(manifest.Source.Remote) == "" {
		result.addError("missing_source_remote", "git source requires a remote URL", "manifest.yaml")
	}
	if strings.TrimSpace(manifest.Source.Remote) != "" {
		if _, err := normalizePullRemoteURL(manifest.Source.Remote); err != nil {
			result.addError("invalid_source_remote", err.Error(), "source.remote")
		}
	}
	layers := append(append([]validationLayer{}, manifest.Source.Layers...), manifest.Source.Overlays...)
	hasLayers := len(layers) > 0
	if sourceType == "git" && hasLayers {
		result.addError("incompatible_source_layers", "git source cannot declare source layers; use type: layered", "manifest.yaml")
	}
	if sourceType == "layered" && !hasLayers {
		result.addError("missing_source_layers", "layered source requires at least one layer", "manifest.yaml")
	}
	seenLayerNames := map[string]struct{}{}
	for i, layer := range layers {
		validateLayer(&result, i, layer, seenLayerNames)
	}

	validateTargetConfigs(&result, manifest.Targets, "manifest.yaml")

	localNames := map[string]struct{}{}
	if sourceType != "git" {
		seenNames := map[string]string{}
		seenPaths := map[string]string{}
		for _, entry := range manifest.Skills {
			localNames[entry.Name] = struct{}{}
			s.validateEntryAt(&result, s.localSourceRoot(manifest.Source.Path), "skill", entry.Name, entry.Path, seenNames, seenPaths)
		}
		for _, entry := range manifest.Configs {
			localNames[entry.Name] = struct{}{}
			s.validateEntryAt(&result, s.localSourceRoot(manifest.Source.Path), "config", entry.Name, entry.Path, seenNames, seenPaths)
		}
	}

	// The same resolved SourceReader powers sync/diff and is intentionally used
	// here so a remote layer cannot validate successfully while sync reads a
	// different source graph.
	if sourceType == "local" || sourceType == "layered" || sourceType == "git" {
		resolvedLayered := sourceType == "layered" || hasLayers
		if source, openErr := s.openSource(ctx); openErr != nil {
			if resolvedLayered || sourceType == "git" {
				result.addError("layered_source_unavailable", openErr.Error(), "source")
			}
		} else {
			defer source.close()
			_, readErr := source.reader.ReadManifest(ctx)
			if readErr != nil && (resolvedLayered || sourceType == "git") {
				result.addError("layered_source_unavailable", readErr.Error(), "source")
			}
			if resolvedLayered || sourceType == "git" {
				for i, layerReader := range source.layers {
					if !sourceTypeIsDirectRemote(sourceType) && i == len(source.layers)-1 {
						continue
					}
					layerManifest, layerErr := layerReader.ReadManifest(ctx)
					if layerErr != nil {
						result.addError("layered_source_unavailable", layerErr.Error(), fmt.Sprintf("source.layers[%d]", i))
						continue
					}
					s.validateResolvedEntries(ctx, &result, layerReader, layerManifest)
				}
			}
		}
	}

	result.Valid = len(result.Errors) == 0
	return result, nil
}

func validateLayer(result *ValidationResult, index int, layer validationLayer, seenNames map[string]struct{}) {
	path := fmt.Sprintf("source.layers[%d]", index)
	name := strings.TrimSpace(layer.Name)
	if name == "" {
		result.addError("empty_source_layer_name", "source layer name is required", path)
	} else if _, ok := seenNames[name]; ok {
		result.addError("duplicate_source_layer_name", fmt.Sprintf("source layer %q is declared more than once", name), path)
	} else {
		seenNames[name] = struct{}{}
	}
	layerType := layer.Type
	if layerType != "local" && layerType != "git" {
		result.addError("unknown_source_layer_type", fmt.Sprintf("source layer %q type %q is unsupported", name, layer.Type), path)
	}
	layerPath := layer.Path
	if layerPath == "" {
		layerPath = ".creed"
	}
	if _, err := safeSourcePath(layerPath); err != nil {
		result.addError("unsafe_source_layer_path", fmt.Sprintf("source layer %q path %q is invalid: %v", name, layerPath, err), path+".path")
	}
	if layerType == "git" && strings.TrimSpace(layer.Remote) == "" {
		result.addError("missing_source_layer_remote", fmt.Sprintf("source layer %q requires a remote URL", name), path)
	}
	if strings.TrimSpace(layer.Remote) != "" {
		if _, err := normalizePullRemoteURL(layer.Remote); err != nil {
			result.addError("invalid_source_layer_remote", err.Error(), path+".remote")
		}
	}
	if strings.ContainsAny(layer.Ref, "\r\n") {
		result.addError("invalid_source_layer_ref", fmt.Sprintf("source layer %q ref must be a single line", name), path+".ref")
	}
}

func sourceTypeIsDirectRemote(sourceType string) bool {
	return sourceType == "git"
}

func validateTargetConfigs(result *ValidationResult, targets []validationTarget, pathPrefix string) {
	seen := map[string]struct{}{}
	for _, target := range targets {
		path := pathPrefix + ".targets"
		if target.Name == "" {
			result.addError("empty_target_name", "target name is required", path)
			continue
		}
		if _, ok := seen[target.Name]; ok {
			result.addError("duplicate_target_name", fmt.Sprintf("target %q is declared more than once", target.Name), path)
		} else {
			seen[target.Name] = struct{}{}
		}
		known, lookupErr := domain.LookupTarget(target.Name)
		if lookupErr != nil {
			result.addError("unknown_target", lookupErr.Error(), path)
			continue
		}
		if target.Enabled && len(known.Outputs("")) == 0 {
			result.addWarning("enabled_target_has_no_outputs", fmt.Sprintf("enabled target %q has no configured outputs", target.Name), path)
		}
		if target.OutputDir == "" || target.OutputDir == "." {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(target.OutputDir))
		if filepath.IsAbs(target.OutputDir) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			result.addError("unsafe_target_output_dir", fmt.Sprintf("target %q output_dir %q escapes the project root", target.Name, target.OutputDir), path)
		}
	}
}

func (s *Implementation) validateResolvedEntries(ctx context.Context, result *ValidationResult, source interface {
	ReadSkill(context.Context, string) (*domain.Skill, error)
	ReadConfig(context.Context, string) (*domain.ConfigFile, error)
}, manifest *domain.Manifest) {
	if manifest == nil {
		result.addError("invalid_layer_manifest", "resolved source returned a nil manifest", "source")
		return
	}
	validateResolvedTargets(result, manifest.Targets)
	seenNames := map[string]string{}
	seenPaths := map[string]string{}
	for _, entry := range manifest.Skills {
		recordResolvedEntry(result, "skill", entry.Name, entry.Path, seenNames, seenPaths)
		if err := validateSkillIdentifier(entry.Name); err != nil {
			continue
		}
		skill, err := source.ReadSkill(ctx, entry.Name)
		if err != nil || skill == nil {
			result.addError("missing_layer_source_file", fmt.Sprintf("skill %q cannot be read from the resolved source: %v", entry.Name, err), entry.Path)
		}
	}
	for _, entry := range manifest.Configs {
		recordResolvedEntry(result, "config", entry.Name, entry.Path, seenNames, seenPaths)
		config, err := source.ReadConfig(ctx, entry.Name)
		if err != nil || config == nil {
			result.addError("missing_layer_source_file", fmt.Sprintf("config %q cannot be read from the resolved source: %v", entry.Name, err), entry.Path)
		}
	}
}

func validateResolvedTargets(result *ValidationResult, targets []domain.TargetConfig) {
	converted := make([]validationTarget, 0, len(targets))
	for _, target := range targets {
		converted = append(converted, validationTarget{Name: target.Name, Enabled: target.Enabled, OutputDir: target.OutputDir})
	}
	validateTargetConfigs(result, converted, "source layer")
}

func recordResolvedEntry(result *ValidationResult, kind, name, sourcePath string, seenNames, seenPaths map[string]string) {
	label := kind + " " + fmt.Sprintf("%q", name)
	if previous, ok := seenNames[name]; ok {
		result.addError("duplicate_source_name", fmt.Sprintf("%s duplicates %s within one source layer", label, previous), sourcePath)
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
}

func (s *Implementation) readValidationManifest(ctx context.Context) (*validationManifest, error) {
	// Read through LocalFS first so a symlinked manifest cannot bypass source
	// containment before the strict YAML decoder below examines its fields.
	data, err := localfs.NewSource(s.root).ReadManifestBytes(ctx)
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

func (s *Implementation) validateEntryAt(result *ValidationResult, sourceRoot, kind, name, sourcePath string, seenNames, seenPaths map[string]string) {
	label := kind + " " + fmt.Sprintf("%q", name)
	if strings.TrimSpace(name) == "" {
		result.addError("empty_source_name", kind+" name is required", sourcePath)
	} else {
		if kind == "skill" {
			if err := validateSkillIdentifier(name); err != nil {
				result.addError("unsafe_source_name", fmt.Sprintf("%s name %q is invalid: %v", label, name, err), sourcePath)
			}
		}
		if previous, ok := seenNames[name]; ok {
			result.addError("duplicate_source_name", fmt.Sprintf("%s duplicates %s", label, previous), sourcePath)
		} else {
			seenNames[name] = label
		}
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

	path := filepath.Join(sourceRoot, cleanPath)
	rootInfo, rootErr := os.Lstat(sourceRoot)
	if rootErr != nil {
		result.addError("unreadable_source_file", fmt.Sprintf("%s source directory cannot be inspected", label), cleanPath)
		return
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		result.addError("symlink_source_file", fmt.Sprintf("%s source directory must not be a symlink", label), cleanPath)
		return
	}
	current := sourceRoot
	parts := strings.Split(cleanPath, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		component, componentErr := os.Lstat(current)
		if os.IsNotExist(componentErr) {
			break
		}
		if componentErr != nil {
			result.addError("unreadable_source_file", fmt.Sprintf("%s source path cannot be inspected", label), cleanPath)
			return
		}
		if component.Mode()&os.ModeSymlink != 0 {
			result.addError("symlink_source_file", fmt.Sprintf("%s source must not traverse a symlink", label), cleanPath)
			return
		}
		if i < len(parts)-1 && !component.IsDir() {
			result.addError("unreadable_source_file", fmt.Sprintf("%s source parent is not a directory", label), cleanPath)
			return
		}
	}
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
	resolvedRoot, resolvedRootErr := filepath.EvalSymlinks(sourceRoot)
	resolvedPath, pathErr := filepath.EvalSymlinks(path)
	if resolvedRootErr != nil || pathErr != nil {
		result.addError("unreadable_source_file", fmt.Sprintf("%s source path cannot be resolved", label), cleanPath)
		return
	}
	if relative, relErr := filepath.Rel(resolvedRoot, resolvedPath); relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		result.addError("escaped_source_path", fmt.Sprintf("%s resolves outside source directory", label), cleanPath)
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

func validateSkillIdentifier(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}
	if filepath.IsAbs(filepath.FromSlash(name)) || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("name must be a single relative path component")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || clean != filepath.FromSlash(name) {
		return fmt.Errorf("name must be a single relative path component")
	}
	return nil
}

func (s *Implementation) localSourceRoot(_ string) string {
	return s.creedDir()
}

func safeSourcePath(sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(sourcePath) || filepath.VolumeName(sourcePath) != "" {
		return "", fmt.Errorf("path must be relative")
	}
	clean := filepath.Clean(filepath.FromSlash(sourcePath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must remain inside source")
	}
	return clean, nil
}

func (r *ValidationResult) addError(code, message, path string) {
	r.Errors = append(r.Errors, ValidationDiagnostic{Severity: "error", Code: code, Message: message, Path: path})
}

func (r *ValidationResult) addWarning(code, message, path string) {
	r.Warnings = append(r.Warnings, ValidationDiagnostic{Severity: "warning", Code: code, Message: message, Path: path})
}
