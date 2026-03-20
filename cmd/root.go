package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/user/skill-sync/internal/config"
	"github.com/user/skill-sync/internal/provider"
)

var (
	cfgPath string
	// Cfg holds the resolved configuration used by all subcommands.
	Cfg           *config.Config
	inlineSource  string
	inlineTargets []string
	sourceDir     string
	targetDir     string
)

var rootCmd = &cobra.Command{
	Use:          "skill-sync",
	Short:        "Sync AI skills across providers",
	SilenceUsage: true,
	Long: `skill-sync reads skills from a source AI assistant (Claude Code, Copilot,
Gemini CLI, Factory) and syncs them to target providers with drift detection.

All providers use the Agent Skills open standard (SKILL.md format).

Configure once with 'skill-sync init', then run 'skill-sync sync' to
keep all your providers in lockstep.`,
	Example: `  # Quick start: init + sync
  skill-sync init --source claude --targets copilot,gemini
  skill-sync sync

  # Check if targets have drifted
  skill-sync status

  # See exactly what changed
  skill-sync diff copilot

  # Override source directory
  skill-sync sync --source-dir ~/.claude/skills

  # Override target directory (single target only)
  skill-sync sync --source claude --targets copilot --target-dir /path/to/skills`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if cmd.Name() == "init" {
			return nil
		}

		var cfg *config.Config
		var err error

		if inlineSource != "" || (sourceDir != "" && targetDir != "") {
			cfg, err = buildConfigFromFlags()
		} else {
			cfg, err = loadConfigFromFile()
		}
		if err != nil {
			return err
		}

		cfg.NormalizeDirectoryMode()
		Cfg = cfg
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", ".skill-sync.yaml", "path to config file")
	rootCmd.PersistentFlags().StringVar(&inlineSource, "source", "", "source provider to read skills from (overrides config file)")
	rootCmd.PersistentFlags().StringSliceVar(&inlineTargets, "targets", nil, "target providers to sync skills to (overrides config file)")
	rootCmd.PersistentFlags().StringVar(&sourceDir, "source-dir", "", "override source provider skill directory")
	rootCmd.PersistentFlags().StringVar(&targetDir, "target-dir", "", "target directory path; use alone for directory mode or with --targets to override a single target's dir")
}

// buildConfigFromFlags creates a Config from CLI flags (--source, --targets, --target-dir, --source-dir).
func buildConfigFromFlags() (*config.Config, error) {
	if len(inlineTargets) == 0 && targetDir == "" {
		return nil, fmt.Errorf("--targets or --target-dir is required when using --source. Example: --source claude --targets copilot,gemini")
	}

	// Pure directory mode: --source-dir + --target-dir, no provider names needed.
	source := inlineSource
	if source == "" && sourceDir != "" {
		source = "directory"
	}

	cfg := &config.Config{
		Source:    source,
		SourceDir: sourceDir,
	}

	// Directory mode: --source claude --target-dir ~/backup
	if targetDir != "" && len(inlineTargets) == 0 {
		cfg.TargetDir = targetDir
	} else {
		cfg.Targets = inlineTargets
	}

	// --target-dir as single-target override: --targets copilot --target-dir /path
	if targetDir != "" && len(inlineTargets) > 0 {
		if len(inlineTargets) > 1 {
			return nil, fmt.Errorf("--target-dir can only be used with a single target (got %d targets)", len(inlineTargets))
		}
		cfg.TargetDirs = map[string]string{inlineTargets[0]: targetDir}
	}

	if err := cfg.Validate(provider.List()); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return cfg, nil
}

// loadConfigFromFile loads a Config from .skill-sync.yaml with CLI flag overrides applied.
func loadConfigFromFile() (*config.Config, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if err := cfg.Validate(provider.List()); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	if sourceDir != "" {
		cfg.SourceDir = sourceDir
	}

	if err := applytargetDirOverride(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applytargetDirOverride applies the --target-dir CLI flag to a file-loaded config.
func applytargetDirOverride(cfg *config.Config) error {
	if targetDir == "" {
		return nil
	}

	// Directory mode: override the path.
	if cfg.TargetDir != "" {
		cfg.TargetDir = targetDir
		return nil
	}

	// Provider mode: override a single target's dir.
	if len(cfg.Targets) > 1 {
		return fmt.Errorf("--target-dir can only be used with a single target (got %d targets)", len(cfg.Targets))
	}
	if cfg.TargetDirs == nil {
		cfg.TargetDirs = make(map[string]string)
	}
	cfg.TargetDirs[cfg.Targets[0]] = targetDir
	return nil
}

// Execute runs the root command.
func Execute() {
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
