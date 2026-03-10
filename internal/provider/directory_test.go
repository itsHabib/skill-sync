package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryProvider_Registration(t *testing.T) {
	// The directory provider is registered via init() in directory.go.
	// Verify it can be created via the registry.
	dir := t.TempDir()
	p, err := New("directory", dir)
	if err != nil {
		t.Fatalf("New(directory) error: %v", err)
	}
	if p.Name() != "directory" {
		t.Errorf("Name() = %q, want %q", p.Name(), "directory")
	}
	if p.SkillDir() != dir {
		t.Errorf("SkillDir() = %q, want %q", p.SkillDir(), dir)
	}
}

func TestDirectoryProvider_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	p, err := New("directory", dir)
	if err != nil {
		t.Fatalf("New(directory) error: %v", err)
	}

	content := "# Deploy\nRun the deploy script with $ARGUMENTS"
	skill := Skill{
		Name:    "deploy",
		Content: content,
	}
	if err := p.WriteSkill(skill); err != nil {
		t.Fatalf("WriteSkill() error: %v", err)
	}

	// Verify file exists at expected path
	path := filepath.Join(dir, "deploy", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	if string(data) != content {
		t.Errorf("file content = %q, want %q", string(data), content)
	}

	// Read it back
	got, err := p.ReadSkill("deploy")
	if err != nil {
		t.Fatalf("ReadSkill() error: %v", err)
	}
	if got.Name != "deploy" {
		t.Errorf("Name = %q, want %q", got.Name, "deploy")
	}
	if got.Content != content {
		t.Errorf("Content = %q, want %q", got.Content, content)
	}
}

func TestDirectoryProvider_ListSkills(t *testing.T) {
	dir := t.TempDir()
	p, err := New("directory", dir)
	if err != nil {
		t.Fatalf("New(directory) error: %v", err)
	}

	// Write two skills
	for _, name := range []string{"alpha", "beta"} {
		if err := p.WriteSkill(Skill{Name: name, Content: "# " + name}); err != nil {
			t.Fatalf("WriteSkill(%s) error: %v", name, err)
		}
	}

	skills, err := p.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills() error: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("ListSkills() returned %d skills, want 2", len(skills))
	}

	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	for _, want := range []string{"alpha", "beta"} {
		if !names[want] {
			t.Errorf("ListSkills() missing skill %q", want)
		}
	}
}
