// Package config handles parsing and validation of .skill-sync.yaml configuration files.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the .skill-sync.yaml configuration file.
type Config struct {
	// Source is the provider name to read skills from (e.g., "claude").
	Source string `yaml:"source"`

	// SourceDir optionally overrides the source provider's default skill directory.
	SourceDir string `yaml:"source_dir,omitempty"`

	// Targets lists provider names to sync skills to (e.g., ["copilot", "gemini"]).
	// Mutually exclusive with TargetDir.
	Targets []string `yaml:"targets,omitempty"`

	// TargetDirs optionally overrides target providers' default skill directories.
	// Keys are provider names, values are directory paths.
	TargetDirs map[string]string `yaml:"target_dirs,omitempty"`

	// TargetDir is the path to a plain directory target (e.g., a git repo for backups).
	// Mutually exclusive with Targets. When set, skills are synced as-is using SKILL.md format.
	TargetDir string `yaml:"target_dir,omitempty"`

	// Skills optionally restricts syncing to the named skills.
	// An empty list means all skills are synced. Uses YAML flow style for compact inline lists.
	Skills []string `yaml:"skills,flow"`
}

// Load reads and parses a .skill-sync.yaml file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: reading file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parsing yaml: %w", err)
	}
	return &cfg, nil
}

// Validate checks that source and target names exist in the provided list
// of registered provider names.
func (c *Config) Validate(registeredNames []string) error {
	nameSet := make(map[string]bool, len(registeredNames))
	for _, n := range registeredNames {
		nameSet[n] = true
	}

	var errs []string
	errs = c.validateSource(errs, nameSet)
	errs = c.validateTargets(errs, nameSet)

	if len(errs) > 0 {
		return fmt.Errorf("config: validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (c *Config) validateSource(errs []string, nameSet map[string]bool) []string {
	if c.Source == "" {
		return append(errs, "source must not be empty")
	}
	// "directory" is a virtual provider for pure dir-to-dir mode (--source-dir + --target-dir).
	if c.Source == "directory" && c.SourceDir != "" {
		return errs
	}
	if !nameSet[c.Source] {
		return append(errs, fmt.Sprintf("unknown source provider %q", c.Source))
	}
	return errs
}

func (c *Config) validateTargets(errs []string, nameSet map[string]bool) []string {
	hasTargets := len(c.Targets) > 0
	hasTargetDir := c.TargetDir != ""

	if hasTargets && hasTargetDir {
		errs = append(errs, "targets and target_dir are mutually exclusive; use one or the other")
	}
	if !hasTargets && !hasTargetDir {
		errs = append(errs, "either targets or target_dir must be specified")
	}

	if hasTargetDir {
		return c.validateDirectoryMode(errs)
	}
	return c.validateProviderMode(errs, nameSet)
}

func (c *Config) validateDirectoryMode(errs []string) []string {
	if len(c.TargetDirs) > 0 {
		errs = append(errs, "target_dirs cannot be used with target_dir")
	}
	return errs
}

func (c *Config) validateProviderMode(errs []string, nameSet map[string]bool) []string {
	targetSet := make(map[string]bool, len(c.Targets))
	for _, t := range c.Targets {
		targetSet[t] = true
		if !nameSet[t] {
			errs = append(errs, fmt.Sprintf("unknown target provider %q", t))
		}
		if t == c.Source {
			errs = append(errs, fmt.Sprintf("source %q must not appear in targets", c.Source))
		}
	}

	for name := range c.TargetDirs {
		if !targetSet[name] {
			errs = append(errs, fmt.Sprintf("target_dirs: %q is not in targets list", name))
		}
	}
	return errs
}

// NormalizeDirectoryMode converts target_dir shorthand into the standard
// targets + target_dirs format so downstream code works unchanged.
// Call this after Validate.
func (c *Config) NormalizeDirectoryMode() {
	if c.TargetDir != "" && len(c.Targets) == 0 {
		c.Targets = []string{"directory"}
		c.TargetDirs = map[string]string{"directory": c.TargetDir}
	}
}
