package provider

import (
	"errors"
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

func TestCodexProviderFailsWhenHomeCannotBeResolved(t *testing.T) {
	want := errors.New("home unavailable")
	p := newCodexProvider("", func() (string, error) {
		return "", want
	})

	if !filepath.IsAbs(p.SkillDir()) && p.SkillDir() != "" {
		t.Fatalf("SkillDir() = %q, want empty or absolute", p.SkillDir())
	}
	_, err := p.ListSkills()
	if !errors.Is(err, want) {
		t.Fatalf("ListSkills() error = %v, want wrapped %v", err, want)
	}
}

func TestCodexProviderFailsWhenHomeIsEmpty(t *testing.T) {
	p := newCodexProvider("", func() (string, error) {
		return "", nil
	})

	_, err := p.ListSkills()
	if err == nil || err.Error() != "codex: resolve user home: empty path" {
		t.Fatalf("ListSkills() error = %v, want empty-path error", err)
	}
}
