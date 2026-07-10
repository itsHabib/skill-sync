package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/itsHabib/skill-sync/internal/provider"
	"github.com/itsHabib/skill-sync/internal/sync"
)

func TestStatusAllInSync(t *testing.T) {
	source := newMockSource(
		provider.Skill{Name: "deploy", Content: "deploy content"},
	)
	target := &mockProvider{
		name:   "copilot",
		skills: []provider.Skill{{Name: "deploy"}},
		readMap: map[string]*provider.Skill{
			"deploy": {Name: "deploy", Content: "deploy content"},
		},
	}

	engine := sync.NewDiffEngine(source, []provider.Provider{target})

	var buf bytes.Buffer
	err := doStatus(&buf, engine, nil)
	if err != nil {
		t.Fatalf("expected no error for all in-sync, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[ok] in-sync") {
		t.Errorf("expected '[ok] in-sync' in output, got: %s", output)
	}
}

func TestStatusMixedDrift(t *testing.T) {
	source := newMockSource(
		provider.Skill{Name: "deploy", Content: "deploy content"},
		provider.Skill{Name: "review", Content: "review v2"},
	)
	target := &mockProvider{
		name:   "copilot",
		skills: []provider.Skill{{Name: "deploy"}, {Name: "review"}},
		readMap: map[string]*provider.Skill{
			"deploy": {Name: "deploy", Content: "deploy content"},
			"review": {Name: "review", Content: "review v1"},
		},
	}

	engine := sync.NewDiffEngine(source, []provider.Provider{target})

	var buf bytes.Buffer
	err := doStatus(&buf, engine, nil)
	if err == nil {
		t.Fatal("expected error for drift detected")
	}

	output := buf.String()
	if !strings.Contains(output, "[ok] in-sync") {
		t.Error("expected '[ok] in-sync' for deploy")
	}
	if !strings.Contains(output, "[!] modified") {
		t.Error("expected '[!] modified' for review")
	}
}

func TestStatusMissingInTarget(t *testing.T) {
	source := newMockSource(
		provider.Skill{Name: "deploy", Content: "deploy content"},
	)
	target := &mockProvider{
		name:    "copilot",
		skills:  []provider.Skill{},
		readMap: map[string]*provider.Skill{},
	}

	engine := sync.NewDiffEngine(source, []provider.Provider{target})

	var buf bytes.Buffer
	err := doStatus(&buf, engine, nil)
	if err == nil {
		t.Fatal("expected error for drift detected")
	}

	output := buf.String()
	if !strings.Contains(output, "[-] missing") {
		t.Errorf("expected '[-] missing' in output, got: %s", output)
	}
}

func TestStatusExtraInTarget(t *testing.T) {
	source := newMockSource()
	target := &mockProvider{
		name:   "copilot",
		skills: []provider.Skill{{Name: "extra-skill"}},
		readMap: map[string]*provider.Skill{
			"extra-skill": {Name: "extra-skill", Content: "extra"},
		},
	}

	engine := sync.NewDiffEngine(source, []provider.Provider{target})

	var buf bytes.Buffer
	err := doStatus(&buf, engine, nil)
	if err == nil {
		t.Fatal("expected error for drift detected")
	}

	output := buf.String()
	if !strings.Contains(output, "[+] extra") {
		t.Errorf("expected '[+] extra' in output, got: %s", output)
	}
}

func TestStatusMultipleTargets(t *testing.T) {
	source := newMockSource(
		provider.Skill{Name: "deploy", Content: "deploy content"},
	)
	target1 := &mockProvider{
		name:   "copilot",
		skills: []provider.Skill{{Name: "deploy"}},
		readMap: map[string]*provider.Skill{
			"deploy": {Name: "deploy", Content: "deploy content"},
		},
	}
	target2 := &mockProvider{
		name:    "gemini",
		skills:  []provider.Skill{},
		readMap: map[string]*provider.Skill{},
	}

	engine := sync.NewDiffEngine(source, []provider.Provider{target1, target2})

	var buf bytes.Buffer
	err := doStatus(&buf, engine, nil)
	if err == nil {
		t.Fatal("expected error for drift detected")
	}

	output := buf.String()
	if !strings.Contains(output, "Target: copilot") {
		t.Error("expected 'Target: copilot' in output")
	}
	if !strings.Contains(output, "Target: gemini") {
		t.Error("expected 'Target: gemini' in output")
	}
}

func TestStatusJSONIsStableAndReportsDrift(t *testing.T) {
	source := newMockSource(
		provider.Skill{Name: "deploy", Content: "deploy content"},
	)
	targets := []provider.Provider{
		&mockProvider{name: "gemini", skills: []provider.Skill{}, readMap: map[string]*provider.Skill{}},
		&mockProvider{
			name:   "codex",
			skills: []provider.Skill{{Name: "deploy"}},
			readMap: map[string]*provider.Skill{
				"deploy": {Name: "deploy", Content: "different"},
			},
		},
	}

	var buf bytes.Buffer
	err := doStatusJSON(&buf, sync.NewDiffEngine(source, targets), nil)
	if err == nil {
		t.Fatal("expected error for drift detected")
	}

	var got jsonStatusReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}
	if !got.Drift {
		t.Fatal("Drift = false, want true")
	}
	if len(got.Targets) != 2 || got.Targets[0].Name != "codex" || got.Targets[1].Name != "gemini" {
		t.Fatalf("Targets = %#v, want codex then gemini", got.Targets)
	}
	if got.Targets[0].Skills[0].Status != "modified" {
		t.Errorf("codex status = %q, want modified", got.Targets[0].Skills[0].Status)
	}
	if got.Targets[1].Skills[0].Status != "missing-in-target" {
		t.Errorf("gemini status = %q, want missing-in-target", got.Targets[1].Skills[0].Status)
	}
}

func TestStatusJSONTreatsUnmanagedAsOwnedButManualAsDrift(t *testing.T) {
	unmanaged := &sync.DriftReport{Results: map[string][]sync.SkillDrift{
		"codex": {{SkillName: "external", Status: provider.Unmanaged}},
	}}
	var buf bytes.Buffer
	if err := writeStatusJSON(&buf, unmanaged, nil); err != nil {
		t.Fatalf("unmanaged entry caused drift: %v", err)
	}
	var report jsonStatusReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Drift {
		t.Fatal("unmanaged entry set drift=true")
	}

	manual := &sync.DriftReport{Results: map[string][]sync.SkillDrift{
		"codex": {{SkillName: "needs-decision", Status: provider.Manual}},
	}}
	buf.Reset()
	if err := writeStatusJSON(&buf, manual, nil); err == nil {
		t.Fatal("manual entry did not cause drift")
	}
}
