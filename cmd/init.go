package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/itsHabib/skill-sync/internal/config"
	"github.com/itsHabib/skill-sync/internal/provider"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a .skill-sync.yaml config file",
	Long: `Generates a .skill-sync.yaml in the current directory declaring which
provider is your source of truth and which providers to sync to.

Run this once per project. Requires --source and --targets flags.`,
	Example: `  # Initialize with Claude as source, sync to Copilot and Gemini
  skill-sync init --source claude --targets copilot,gemini

  # Initialize with all targets
  skill-sync init --source claude --targets copilot,gemini,factory

  # Initialize for directory backup (e.g., a git repo)
  skill-sync init --source claude --target-dir ~/dev/cc-skills

  # Use a custom config path
  skill-sync init --source claude --targets copilot --config my-config.yaml`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, _ []string) error {
	if inlineSource == "" {
		return fmt.Errorf("--source is required. Specify your source provider: skill-sync init --source claude --targets copilot,gemini")
	}
	if len(inlineTargets) == 0 && firstTargetDir() == "" {
		return fmt.Errorf("--targets or --target-dir is required. Examples:\n  skill-sync init --source claude --targets copilot,gemini\n  skill-sync init --source claude --target-dir ~/dev/cc-skills")
	}

	// Check if config already exists
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("%s already exists; remove it first or use a different --config path", cfgPath)
	}

	// Build config from flags
	registered := provider.List()
	cfg := &config.Config{
		Source: inlineSource,
		Skills: []string{},
	}
	if firstTargetDir() != "" && len(inlineTargets) == 0 {
		cfg.TargetDir = firstTargetDir()
	} else {
		cfg.Targets = inlineTargets
	}
	if err := cfg.Validate(registered); err != nil {
		return fmt.Errorf("init: %w", err)
	}

	// Marshal and write config
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("init: marshaling config: %w", err)
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("init: writing config: %w", err)
	}

	w := cmd.OutOrStdout()
	if cfg.TargetDir != "" {
		fmt.Fprintf(w, "Created %s (source: %s, target_dir: %s)\n", cfgPath, cfg.Source, cfg.TargetDir)
	} else {
		fmt.Fprintf(w, "Created %s (source: %s, targets: %v)\n", cfgPath, cfg.Source, cfg.Targets)
	}
	return nil
}
