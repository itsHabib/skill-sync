package catalog

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/itsHabib/skill-sync/internal/provider"
)

const propertySeed int64 = 20260710

func TestStatusClassifiesManagedManualUnmanagedAndExtra(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "skills/alpha", "alpha")
	writeSkill(t, root, "targets/codex/beta", "beta-source")
	writeSkill(t, root, "skills/gamma", "gamma")
	manifest := writeManifest(t, root, `version: 1
skills:
  alpha:
    owner: michael
    visibility: private
    mode: portable-copy
    source: skills/alpha
    targets: [codex]
  beta:
    owner: michael
    visibility: private
    mode: target-adapted
    sources: {codex: targets/codex/beta}
    targets: [codex]
  gamma:
    owner: michael
    visibility: private
    mode: replacement
    source: skills/gamma
    targets: [codex]
  manual-one:
    owner: michael
    visibility: private
    mode: manual
    targets: [codex]
unmanaged:
  codex: [external]
`)

	targetDir := t.TempDir()
	writeSkill(t, targetDir, "alpha", "alpha")
	writeSkill(t, targetDir, "beta", "beta-installed")
	writeSkill(t, targetDir, "manual-one", "manual")
	writeSkill(t, targetDir, "external", "external")
	writeSkill(t, targetDir, "unknown", "unknown")
	target := mustProvider(t, targetDir)

	c := mustLoad(t, root, manifest)
	report, err := c.Status([]provider.Provider{target})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]provider.SkillStatus{}
	for _, drift := range report.Results["codex"] {
		got[drift.SkillName] = drift.Status
	}
	want := map[string]provider.SkillStatus{
		"alpha":      provider.InSync,
		"beta":       provider.Modified,
		"gamma":      provider.MissingInTarget,
		"manual-one": provider.Manual,
		"external":   provider.Unmanaged,
		"unknown":    provider.ExtraInTarget,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %#v, want %#v", got, want)
	}
}

func TestSyncRefusesConflictThenForceConvergesIdempotently(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "skills/alpha", "alpha-source")
	writeSkill(t, root, "skills/beta", "beta-source")
	manifest := writeManifest(t, root, portableManifest("alpha", "beta"))
	targetDir := t.TempDir()
	writeSkill(t, targetDir, "beta", "beta-installed")
	target := mustProvider(t, targetDir)
	c := mustLoad(t, root, manifest)

	result, err := c.Sync(target, false, false, nil)
	if err == nil || result.Errors != 1 {
		t.Fatalf("first sync error/result = %v/%+v, want one conflict", err, result)
	}
	if got := readSkillContent(t, targetDir, "beta"); got != skillDocument("beta", "beta-installed") {
		t.Fatalf("conflicting target overwritten without force: %q", got)
	}

	result, err = c.Sync(target, false, true, nil)
	if err != nil || result.Errors != 0 {
		t.Fatalf("forced sync error/result = %v/%+v", err, result)
	}
	first := snapshotTree(t, targetDir)
	result, err = c.Sync(target, false, true, nil)
	if err != nil || result.Errors != 0 {
		t.Fatalf("second sync error/result = %v/%+v", err, result)
	}
	second := snapshotTree(t, targetDir)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("second sync changed converged target: first=%v second=%v", first, second)
	}
	for _, detail := range result.Details {
		if detail.Action != actionInSync {
			t.Fatalf("second sync action for %s = %s, want in-sync", detail.Skill, detail.Action)
		}
	}
}

func TestPropertyStatusAndDryRunNeverMutate(t *testing.T) {
	var propertyErr error
	property := func(seed uint64) bool {
		propertyErr = checkNonMutation(t, seed)
		return propertyErr == nil
	}
	config := &quick.Config{MaxCount: 100, Rand: rand.New(rand.NewSource(propertySeed))}
	if err := quick.Check(property, config); err != nil {
		t.Fatalf("property seed=%d: %v; detail: %v", propertySeed, err, propertyErr)
	}
}

func TestPropertySyncIsIdempotent(t *testing.T) {
	var propertyErr error
	property := func(seed uint64) bool {
		propertyErr = checkIdempotence(t, seed)
		return propertyErr == nil
	}
	config := &quick.Config{MaxCount: 100, Rand: rand.New(rand.NewSource(propertySeed))}
	if err := quick.Check(property, config); err != nil {
		t.Fatalf("property seed=%d: %v; detail: %v", propertySeed, err, propertyErr)
	}
}

func TestPropertySourcesCannotEscapeRoot(t *testing.T) {
	outside := t.TempDir()
	writeSkill(t, outside, "escaped", "outside")
	root := t.TempDir()
	rel, err := filepath.Rel(root, filepath.Join(outside, "escaped"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := writeManifest(t, root, fmt.Sprintf(`version: 1
skills:
  escaped:
    owner: michael
    visibility: private
    mode: portable-copy
    source: %q
    targets: [codex]
`, filepath.ToSlash(rel)))
	if _, err := Load(root, manifest); err == nil {
		t.Fatal("source outside catalog root was accepted")
	}
}

func TestLoadRejectsMissingFrontmatterAndBrokenRelativeLinks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "plain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := writeManifest(t, root, portableManifest("plain"))
	if _, err := Load(root, manifest); err == nil {
		t.Fatal("source without frontmatter was accepted")
	}

	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillDocument("plain", "[missing](templates/nope.md)")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, manifest); err == nil {
		t.Fatal("source with broken relative link was accepted")
	}
}

func TestLoadIgnoresTemplateLinksInsideCode(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "skills/template", "```markdown\n[generated](../../not-in-the-skill.md)\n```\n`[inline]($DYNAMIC)`")
	manifest := writeManifest(t, root, portableManifest("template"))
	if _, err := Load(root, manifest); err != nil {
		t.Fatalf("template link inside code was treated as a source reference: %v", err)
	}
}

func checkNonMutation(t *testing.T, seed uint64) error {
	t.Helper()
	root, targetDir, manifest := generatedCatalog(t, seed)
	target := mustProvider(t, targetDir)
	c := mustLoad(t, root, manifest)
	before := snapshotTree(t, targetDir)
	if _, err := c.Status([]provider.Provider{target}); err != nil {
		return err
	}
	afterStatus := snapshotTree(t, targetDir)
	if !reflect.DeepEqual(before, afterStatus) {
		return fmt.Errorf("status mutated target for generated seed %d", seed)
	}
	_, _ = c.Sync(target, true, true, nil)
	afterDryRun := snapshotTree(t, targetDir)
	if !reflect.DeepEqual(before, afterDryRun) {
		return fmt.Errorf("dry-run mutated target for generated seed %d", seed)
	}
	return nil
}

func checkIdempotence(t *testing.T, seed uint64) error {
	t.Helper()
	root, targetDir, manifest := generatedCatalog(t, seed)
	target := mustProvider(t, targetDir)
	c := mustLoad(t, root, manifest)
	if _, err := c.Sync(target, false, true, nil); err != nil {
		return err
	}
	first := snapshotTree(t, targetDir)
	if _, err := c.Sync(target, false, true, nil); err != nil {
		return err
	}
	second := snapshotTree(t, targetDir)
	if !reflect.DeepEqual(first, second) {
		return fmt.Errorf("sync not idempotent for generated seed %d", seed)
	}
	return nil
}

func generatedCatalog(t *testing.T, seed uint64) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	target := t.TempDir()
	count := int(seed%4) + 1
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("skill-%d", i)
		names = append(names, name)
		writeSkill(t, root, "skills/"+name, fmt.Sprintf("source-%d-%d", seed, i))
		if (seed>>i)&1 == 1 {
			writeSkill(t, target, name, fmt.Sprintf("target-%d-%d", seed, i))
		}
	}
	manifest := writeManifest(t, root, portableManifest(names...))
	return root, target, manifest
}

func portableManifest(names ...string) string {
	content := "version: 1\nskills:\n"
	for _, name := range names {
		content += fmt.Sprintf("  %s:\n    owner: michael\n    visibility: private\n    mode: portable-copy\n    source: skills/%s\n    targets: [codex]\n", name, name)
	}
	return content
}

func writeSkill(t *testing.T, root, relative, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(dir)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillDocument(name, content)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillDocument(name, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: Test skill %s\n---\n%s", name, name, body)
}

func writeManifest(t *testing.T, root, content string) string {
	t.Helper()
	path := filepath.Join(root, "catalog.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustLoad(t *testing.T, root, manifest string) *Catalog {
	t.Helper()
	c, err := Load(root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustProvider(t *testing.T, dir string) provider.Provider {
	t.Helper()
	p, err := provider.New("codex", dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func readSkillContent(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func snapshotTree(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	snapshot := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(rel)] = sha256.Sum256(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
