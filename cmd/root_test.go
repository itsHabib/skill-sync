package cmd

import (
	"testing"
)

// saveAndRestore saves the package-level flag vars and restores them on cleanup.
func saveAndRestore(t *testing.T) {
	t.Helper()
	origSource := inlineSource
	origTargets := inlineTargets
	origSourceDir := sourceDir
	origTargetDirFlags := targetDirFlags
	t.Cleanup(func() {
		inlineSource = origSource
		inlineTargets = origTargets
		sourceDir = origSourceDir
		targetDirFlags = origTargetDirFlags
	})
}

func TestBuildConfigFromFlags_PureDirectoryMode(t *testing.T) {
	saveAndRestore(t)

	inlineSource = ""
	inlineTargets = nil
	sourceDir = "/some/source"
	targetDirFlags = []string{"/some/target"}

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
	if len(cfg.TargetDirList) != 1 || cfg.TargetDirList[0] != "/some/target" {
		t.Errorf("TargetDirList = %v, want [/some/target]", cfg.TargetDirList)
	}
}

func TestBuildConfigFromFlags_MultipleTargetDirs(t *testing.T) {
	saveAndRestore(t)

	inlineSource = ""
	inlineTargets = nil
	sourceDir = "/some/source"
	targetDirFlags = []string{"/target/one", "/target/two", "/target/three"}

	cfg, err := buildConfigFromFlags()
	if err != nil {
		t.Fatalf("buildConfigFromFlags() error = %v", err)
	}

	if cfg.Source != "directory" {
		t.Errorf("Source = %q, want %q", cfg.Source, "directory")
	}
	if len(cfg.TargetDirList) != 3 {
		t.Fatalf("TargetDirList = %v, want 3 entries", cfg.TargetDirList)
	}

	// After normalization, each dir should become a target.
	cfg.NormalizeDirectoryMode()
	if len(cfg.Targets) != 3 {
		t.Fatalf("after normalize, Targets = %v, want 3 entries", cfg.Targets)
	}
	for i, dir := range []string{"/target/one", "/target/two", "/target/three"} {
		if cfg.Targets[i] != dir {
			t.Errorf("Targets[%d] = %q, want %q", i, cfg.Targets[i], dir)
		}
		if cfg.TargetDirs[dir] != dir {
			t.Errorf("TargetDirs[%q] = %q, want %q", dir, cfg.TargetDirs[dir], dir)
		}
	}
}

func TestBuildConfigFromFlags_SourceDirWithoutTargetDir(t *testing.T) {
	saveAndRestore(t)

	inlineSource = ""
	inlineTargets = nil
	sourceDir = "/some/source"
	targetDirFlags = nil

	_, err := buildConfigFromFlags()
	if err == nil {
		t.Fatal("buildConfigFromFlags() expected error when --source-dir set without --target-dir or --targets")
	}
}

func TestBuildConfigFromFlags_InlineSourceStillWorks(t *testing.T) {
	saveAndRestore(t)

	inlineSource = "claude"
	inlineTargets = nil
	sourceDir = ""
	targetDirFlags = []string{"/some/backup"}

	cfg, err := buildConfigFromFlags()
	if err != nil {
		t.Fatalf("buildConfigFromFlags() error = %v", err)
	}

	if cfg.Source != "claude" {
		t.Errorf("Source = %q, want %q", cfg.Source, "claude")
	}
	if len(cfg.TargetDirList) != 1 || cfg.TargetDirList[0] != "/some/backup" {
		t.Errorf("TargetDirList = %v, want [/some/backup]", cfg.TargetDirList)
	}
}
