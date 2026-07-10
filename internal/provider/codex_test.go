package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodexProviderDefaultDirectory(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	p, err := Get("codex")
	if err != nil {
		t.Fatalf("Get(codex): %v", err)
	}

	want := filepath.Join(home, ".codex", "skills")
	if got := p.SkillDir(); got != want {
		t.Errorf("SkillDir() = %q, want %q", got, want)
	}
}

func TestCodexProviderDirectoryOverride(t *testing.T) {
	dir := t.TempDir()
	p, err := New("codex", dir)
	if err != nil {
		t.Fatalf("New(codex): %v", err)
	}
	if got := p.SkillDir(); got != dir {
		t.Errorf("SkillDir() = %q, want %q", got, dir)
	}
}
