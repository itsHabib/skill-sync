package cmd

import (
	"testing"
)

func TestBuildConfigFromFlags_PureDirectoryMode(t *testing.T) {
	// Save and restore package-level vars.
	origSource := inlineSource
	origTargets := inlineTargets
	origSourceDir := sourceDir
	origTargetDir := targetDir
	t.Cleanup(func() {
		inlineSource = origSource
		inlineTargets = origTargets
		sourceDir = origSourceDir
		targetDir = origTargetDir
	})

	// Simulate: skill-sync status --source-dir /a --target-dir /b
	inlineSource = ""
	inlineTargets = nil
	sourceDir = "/some/source"
	targetDir = "/some/target"

	cfg, err := buildConfigFromFlags()
	if err != nil {
		t.Fatalf("buildConfigFromFlags() error = %v", err)
	}

	if cfg.Source != "directory" {
		t.Errorf("Source = %q, want %q", cfg.Source, "directory")
	}
	if cfg.SourceDir != "/some/source" {
		t.Errorf("SourceDir = %q, want %q", cfg.SourceDir, "/some/source")
	}
	if cfg.TargetDir != "/some/target" {
		t.Errorf("TargetDir = %q, want %q", cfg.TargetDir, "/some/target")
	}
}

func TestBuildConfigFromFlags_SourceDirWithoutTargetDir(t *testing.T) {
	origSource := inlineSource
	origTargets := inlineTargets
	origSourceDir := sourceDir
	origTargetDir := targetDir
	t.Cleanup(func() {
		inlineSource = origSource
		inlineTargets = origTargets
		sourceDir = origSourceDir
		targetDir = origTargetDir
	})

	// --source-dir alone without --target-dir or --targets should fail.
	inlineSource = ""
	inlineTargets = nil
	sourceDir = "/some/source"
	targetDir = ""

	_, err := buildConfigFromFlags()
	if err == nil {
		t.Fatal("buildConfigFromFlags() expected error when --source-dir set without --target-dir or --targets")
	}
}

func TestBuildConfigFromFlags_InlineSourceStillWorks(t *testing.T) {
	origSource := inlineSource
	origTargets := inlineTargets
	origSourceDir := sourceDir
	origTargetDir := targetDir
	t.Cleanup(func() {
		inlineSource = origSource
		inlineTargets = origTargets
		sourceDir = origSourceDir
		targetDir = origTargetDir
	})

	// Simulate: skill-sync sync --source claude --target-dir /backup
	inlineSource = "claude"
	inlineTargets = nil
	sourceDir = ""
	targetDir = "/some/backup"

	cfg, err := buildConfigFromFlags()
	if err != nil {
		t.Fatalf("buildConfigFromFlags() error = %v", err)
	}

	if cfg.Source != "claude" {
		t.Errorf("Source = %q, want %q", cfg.Source, "claude")
	}
	if cfg.TargetDir != "/some/backup" {
		t.Errorf("TargetDir = %q, want %q", cfg.TargetDir, "/some/backup")
	}
}
