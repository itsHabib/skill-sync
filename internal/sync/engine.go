// Package sync provides engines for syncing and diffing skills across providers.
package sync

import (
	"fmt"

	"github.com/user/skill-sync/internal/provider"
)

// Status represents the outcome of syncing a single skill to a target.
type Status string

const (
	// StatusSuccess indicates the skill was synced successfully.
	StatusSuccess Status = "success"
	// StatusError indicates the skill failed to sync.
	StatusError Status = "error"
	// StatusSkipped indicates the skill was skipped because it already exists in the target.
	StatusSkipped Status = "skipped"
)

// Detail records the result of syncing one skill to one target.
type Detail struct {
	SkillName string
	// Target is the provider name the skill was synced to.
	Target string
	Status Status
	// Error is non-nil only when Status is StatusError.
	Error error
}

// Result aggregates the outcome of a sync operation.
type Result struct {
	TotalSynced  int
	TotalSkipped int
	TotalErrored int
	// Details contains one entry per (skill, target) pair attempted.
	Details []Detail
}

// Engine orchestrates reading skills from a source provider and writing
// them to one or more target providers.
type Engine struct {
	source  provider.Provider
	targets []provider.Provider
}

// NewEngine creates a sync engine with a source and target providers.
func NewEngine(source provider.Provider, targets []provider.Provider) *Engine {
	return &Engine{
		source:  source,
		targets: targets,
	}
}

// Sync reads skills from the source and writes them to all targets.
// If skillFilter is non-empty, only skills whose names appear in the filter are synced.
// If force is false, skills that already exist in a target are skipped.
// Returns a fatal error only if the source ListSkills call fails.
// Per-skill and per-target errors are captured in Result.Details.
func (e *Engine) Sync(skillFilter []string, force bool) (*Result, error) {
	skills, err := e.source.ListSkills()
	if err != nil {
		return nil, fmt.Errorf("sync: list source skills: %w", err)
	}

	skills = filterSkills(skills, skillFilter)
	existing := e.loadExisting(force)
	result := &Result{}

	for _, skill := range skills {
		full, readErr := e.source.ReadSkill(skill.Name)
		if readErr != nil {
			e.recordReadError(result, skill.Name, readErr)
			continue
		}
		for _, target := range e.targets {
			e.syncSkillToTarget(result, *full, target, existing, force)
		}
	}

	return result, nil
}

func filterSkills(skills []provider.Skill, filter []string) []provider.Skill {
	if len(filter) == 0 {
		return skills
	}
	filterSet := make(map[string]bool, len(filter))
	for _, name := range filter {
		filterSet[name] = true
	}
	filtered := skills[:0:0]
	for _, s := range skills {
		if filterSet[s.Name] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func (e *Engine) loadExisting(force bool) map[string]map[string]bool {
	existing := make(map[string]map[string]bool, len(e.targets))
	if force {
		return existing
	}
	for _, target := range e.targets {
		skills, err := target.ListSkills()
		if err != nil {
			existing[target.Name()] = map[string]bool{}
			continue
		}
		nameSet := make(map[string]bool, len(skills))
		for _, s := range skills {
			nameSet[s.Name] = true
		}
		existing[target.Name()] = nameSet
	}
	return existing
}

func (e *Engine) recordReadError(result *Result, skillName string, err error) {
	for _, target := range e.targets {
		result.Details = append(result.Details, Detail{
			SkillName: skillName,
			Target:    target.Name(),
			Status:    StatusError,
			Error:     fmt.Errorf("sync: read source skill %q: %w", skillName, err),
		})
		result.TotalErrored++
	}
}

func (e *Engine) syncSkillToTarget(result *Result, skill provider.Skill, target provider.Provider, existing map[string]map[string]bool, force bool) {
	if !force {
		if names, ok := existing[target.Name()]; ok && names[skill.Name] {
			result.Details = append(result.Details, Detail{
				SkillName: skill.Name,
				Target:    target.Name(),
				Status:    StatusSkipped,
			})
			result.TotalSkipped++
			return
		}
	}

	if writeErr := target.WriteSkill(skill); writeErr != nil {
		result.Details = append(result.Details, Detail{
			SkillName: skill.Name,
			Target:    target.Name(),
			Status:    StatusError,
			Error:     fmt.Errorf("sync: write skill %q to %q: %w", skill.Name, target.Name(), writeErr),
		})
		result.TotalErrored++
		return
	}

	result.Details = append(result.Details, Detail{
		SkillName: skill.Name,
		Target:    target.Name(),
		Status:    StatusSuccess,
	})
	result.TotalSynced++
}
