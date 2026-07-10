// Package catalog applies an explicit skill ownership manifest to installed
// provider homes. It is the policy layer above provider filesystem mechanics.
package catalog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/itsHabib/skill-sync/internal/provider"
	syncengine "github.com/itsHabib/skill-sync/internal/sync"
	"gopkg.in/yaml.v3"
)

const manifestVersion = 1

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
var inlineCodePattern = regexp.MustCompile("`[^`]*`")

const (
	modePortableCopy  = "portable-copy"
	modeTargetAdapted = "target-adapted"
	modeReplacement   = "replacement"
	modeManual        = "manual"
)

// Manifest declares canonical source paths and explicit target ownership.
type Manifest struct {
	Version   int                 `yaml:"version"`
	Skills    map[string]Skill    `yaml:"skills"`
	Unmanaged map[string][]string `yaml:"unmanaged,omitempty"`
}

// Skill is one catalog entry. Source paths are relative to the catalog root.
type Skill struct {
	Owner      string            `yaml:"owner"`
	Visibility string            `yaml:"visibility"`
	Mode       string            `yaml:"mode"`
	Source     string            `yaml:"source,omitempty"`
	Sources    map[string]string `yaml:"sources,omitempty"`
	Targets    []string          `yaml:"targets"`
}

// Catalog is a validated manifest rooted at a versioned repository directory.
type Catalog struct {
	root     string
	manifest Manifest
}

// Load parses and validates a manifest. root is the boundary all source paths
// must remain inside; manifestPath may be relative to the current directory.
func Load(root, manifestPath string) (*Catalog, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("catalog: resolve root: %w", err)
	}
	absRoot, err = filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("catalog: resolve root symlinks: %w", err)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("catalog: read manifest: %w", err)
	}
	var manifest Manifest
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("catalog: parse manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("catalog: manifest must contain exactly one YAML document")
		}
		return nil, fmt.Errorf("catalog: parse trailing manifest content: %w", err)
	}

	c := &Catalog{root: absRoot, manifest: manifest}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Catalog) validate() error {
	if c.manifest.Version != manifestVersion {
		return fmt.Errorf("catalog: unsupported manifest version %d", c.manifest.Version)
	}
	if len(c.manifest.Skills) == 0 {
		return fmt.Errorf("catalog: skills must not be empty")
	}

	for _, name := range sortedSkillNames(c.manifest.Skills) {
		if err := c.validateSkill(name, c.manifest.Skills[name]); err != nil {
			return err
		}
	}
	return c.validateUnmanaged()
}

func (c *Catalog) validateUnmanaged() error {
	for target, names := range c.manifest.Unmanaged {
		if strings.TrimSpace(target) == "" {
			return fmt.Errorf("catalog: unmanaged target must not be empty")
		}
		seen := map[string]bool{}
		for _, name := range names {
			if name == "" || strings.ContainsAny(name, `/\\`) {
				return fmt.Errorf("catalog: invalid unmanaged skill name %q for %s", name, target)
			}
			if seen[name] {
				return fmt.Errorf("catalog: unmanaged %s/%s declared twice", target, name)
			}
			seen[name] = true
			if skill, ok := c.manifest.Skills[name]; ok && supportsTarget(skill, target) {
				return fmt.Errorf("catalog: %s/%s is both managed and unmanaged", target, name)
			}
		}
	}
	return nil
}

func (c *Catalog) validateSkill(name string, skill Skill) error {
	if err := validateSkillMetadata(name, skill); err != nil {
		return err
	}

	switch skill.Mode {
	case modePortableCopy, modeReplacement:
		return c.validateSharedSource(name, skill)
	case modeTargetAdapted:
		return c.validateTargetSources(name, skill)
	case modeManual:
		if skill.Source != "" || len(skill.Sources) != 0 {
			return fmt.Errorf("catalog: manual skill %s cannot declare sources", name)
		}
		return nil
	default:
		return fmt.Errorf("catalog: skill %s has unknown mode %q", name, skill.Mode)
	}
}

func validateSkillMetadata(name string, skill Skill) error {
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("catalog: invalid skill name %q", name)
	}
	if skill.Owner == "" {
		return fmt.Errorf("catalog: skill %s has no owner", name)
	}
	if skill.Visibility != "private" && skill.Visibility != "public" {
		return fmt.Errorf("catalog: skill %s has invalid visibility %q", name, skill.Visibility)
	}
	if len(skill.Targets) == 0 {
		return fmt.Errorf("catalog: skill %s has no targets", name)
	}
	if err := validateUniqueTargets(name, skill.Targets); err != nil {
		return err
	}
	return nil
}

func (c *Catalog) validateSharedSource(name string, skill Skill) error {
	if skill.Source == "" || len(skill.Sources) != 0 {
		return fmt.Errorf("catalog: skill %s mode %s requires source and no sources map", name, skill.Mode)
	}
	_, err := c.resolveSkillDir(name, skill.Source)
	return err
}

func (c *Catalog) validateTargetSources(name string, skill Skill) error {
	if skill.Source != "" {
		return fmt.Errorf("catalog: skill %s target-adapted mode cannot set source", name)
	}
	for _, target := range skill.Targets {
		source := skill.Sources[target]
		if source == "" {
			return fmt.Errorf("catalog: skill %s has no source for target %s", name, target)
		}
		if _, err := c.resolveSkillDir(name, source); err != nil {
			return err
		}
	}
	if len(skill.Sources) != len(skill.Targets) {
		return fmt.Errorf("catalog: skill %s sources must match targets exactly", name)
	}
	return nil
}

// Status compares every declared target projection and classifies target-only
// skills as unmanaged or unknown extras. It never writes.
func (c *Catalog) Status(targets []provider.Provider) (*syncengine.DriftReport, error) {
	report := &syncengine.DriftReport{Results: make(map[string][]syncengine.SkillDrift, len(targets))}
	for _, target := range targets {
		drifts, err := c.statusTarget(target)
		if err != nil {
			return nil, err
		}
		report.Results[target.Name()] = drifts
	}
	return report, nil
}

func (c *Catalog) statusTarget(target provider.Provider) ([]syncengine.SkillDrift, error) {
	targetSkills, err := target.ListSkills()
	if err != nil {
		return nil, fmt.Errorf("catalog: list target %s: %w", target.Name(), err)
	}
	targetMap := make(map[string]provider.Skill, len(targetSkills))
	for _, skill := range targetSkills {
		targetMap[skill.Name] = skill
	}

	declared := map[string]bool{}
	var drifts []syncengine.SkillDrift
	for _, name := range sortedSkillNames(c.manifest.Skills) {
		skill := c.manifest.Skills[name]
		if !supportsTarget(skill, target.Name()) {
			continue
		}
		declared[name] = true
		if skill.Mode == modeManual {
			drifts = append(drifts, syncengine.SkillDrift{SkillName: name, Status: provider.Manual})
			continue
		}

		source, err := c.readSource(name, skill, target.Name())
		if err != nil {
			return nil, err
		}
		if _, ok := targetMap[name]; !ok {
			drifts = append(drifts, syncengine.SkillDrift{SkillName: name, Status: provider.MissingInTarget})
			continue
		}
		installed, err := target.ReadSkill(name)
		if err != nil {
			return nil, fmt.Errorf("catalog: read target %s/%s: %w", target.Name(), name, err)
		}
		status := provider.Modified
		if syncengine.SkillsMatch(source, installed) {
			status = provider.InSync
		}
		drifts = append(drifts, syncengine.SkillDrift{SkillName: name, Status: status})
	}

	unmanaged := stringSet(c.manifest.Unmanaged[target.Name()])
	for _, installed := range targetSkills {
		if declared[installed.Name] {
			continue
		}
		status := provider.ExtraInTarget
		if unmanaged[installed.Name] {
			status = provider.Unmanaged
		}
		drifts = append(drifts, syncengine.SkillDrift{SkillName: installed.Name, Status: status})
	}
	sort.Slice(drifts, func(i, j int) bool { return drifts[i].SkillName < drifts[j].SkillName })
	return drifts, nil
}

func (c *Catalog) readSource(name string, skill Skill, target string) (*provider.Skill, error) {
	source := skill.Source
	if skill.Mode == modeTargetAdapted {
		source = skill.Sources[target]
	}
	dir, err := c.resolveSkillDir(name, source)
	if err != nil {
		return nil, err
	}
	p, err := provider.New("directory", filepath.Dir(dir))
	if err != nil {
		return nil, fmt.Errorf("catalog: source provider: %w", err)
	}
	got, err := p.ReadSkill(name)
	if err != nil {
		return nil, fmt.Errorf("catalog: read source %s for %s: %w", source, target, err)
	}
	return got, nil
}

func (c *Catalog) resolveSkillDir(name, source string) (string, error) {
	if source == "" || filepath.IsAbs(source) {
		return "", fmt.Errorf("catalog: skill %s source must be a relative path", name)
	}
	joined := filepath.Join(c.root, filepath.FromSlash(source))
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", fmt.Errorf("catalog: resolve source for %s: %w", name, err)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("catalog: absolute source for %s: %w", name, err)
	}
	rel, err := filepath.Rel(c.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("catalog: skill %s source escapes catalog root", name)
	}
	if filepath.Base(abs) != name {
		return "", fmt.Errorf("catalog: skill %s source directory must end in %s", name, name)
	}
	if _, err := os.Stat(filepath.Join(abs, "SKILL.md")); err != nil {
		return "", fmt.Errorf("catalog: skill %s source has no SKILL.md: %w", name, err)
	}
	if err := validateSkillDocument(name, abs); err != nil {
		return "", err
	}
	return abs, nil
}

func validateSkillDocument(name, skillDir string) error {
	path := filepath.Join(skillDir, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("catalog: read %s/SKILL.md: %w", name, err)
	}
	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return fmt.Errorf("catalog: skill %s SKILL.md has no YAML frontmatter", name)
	}
	end := strings.Index(raw[4:], "\n---")
	if end < 0 {
		return fmt.Errorf("catalog: skill %s SKILL.md has unterminated YAML frontmatter", name)
	}
	var frontmatter struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(raw[4:4+end]), &frontmatter); err != nil {
		return fmt.Errorf("catalog: skill %s frontmatter: %w", name, err)
	}
	if frontmatter.Name != name {
		return fmt.Errorf("catalog: skill %s frontmatter name is %q", name, frontmatter.Name)
	}
	if strings.TrimSpace(frontmatter.Description) == "" {
		return fmt.Errorf("catalog: skill %s frontmatter description is empty", name)
	}
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(stripMarkdownCode(raw), -1) {
		if err := validateRelativeLink(skillDir, match[1]); err != nil {
			return fmt.Errorf("catalog: skill %s: %w", name, err)
		}
	}
	return nil
}

func stripMarkdownCode(raw string) string {
	var kept []string
	inFence := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		kept = append(kept, inlineCodePattern.ReplaceAllString(line, ""))
	}
	return strings.Join(kept, "\n")
}

func validateRelativeLink(skillDir, destination string) error {
	destination = strings.Trim(strings.TrimSpace(destination), "<>")
	if fields := strings.Fields(destination); len(fields) > 0 {
		destination = fields[0]
	}
	if destination == "" || strings.HasPrefix(destination, "#") || strings.HasPrefix(destination, "/") || strings.Contains(destination, "://") || strings.HasPrefix(destination, "mailto:") {
		return nil
	}
	if index := strings.IndexAny(destination, "?#"); index >= 0 {
		destination = destination[:index]
	}
	path, err := confinedRelativePath(skillDir, destination)
	if err != nil {
		return fmt.Errorf("relative link %q escapes the skill directory", destination)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("relative link %q is missing: %w", destination, err)
	}
	return nil
}

func confinedRelativePath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path must be non-empty and relative")
	}
	joined := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return joined, nil
}

func supportsTarget(skill Skill, target string) bool {
	for _, candidate := range skill.Targets {
		if candidate == target {
			return true
		}
	}
	return false
}

func validateUniqueTargets(name string, targets []string) error {
	seen := map[string]bool{}
	for _, target := range targets {
		if target == "" {
			return fmt.Errorf("catalog: skill %s has an empty target", name)
		}
		if seen[target] {
			return fmt.Errorf("catalog: skill %s target %s declared twice", name, target)
		}
		seen[target] = true
	}
	return nil
}

func sortedSkillNames(skills map[string]Skill) []string {
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
