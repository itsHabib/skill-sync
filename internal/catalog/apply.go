package catalog

import (
	"errors"
	"fmt"
	"os"

	"github.com/itsHabib/skill-sync/internal/provider"
	syncengine "github.com/itsHabib/skill-sync/internal/sync"
)

// SyncAction describes one declared projection during apply or dry-run.
type SyncAction string

const (
	actionInSync      SyncAction = "in-sync"
	actionCreated     SyncAction = "created"
	actionUpdated     SyncAction = "updated"
	actionWouldCreate SyncAction = "would-create"
	actionWouldUpdate SyncAction = "would-update"
	actionConflict    SyncAction = "conflict"
	actionManual      SyncAction = "manual"
	actionError       SyncAction = "error"
)

// SyncDetail is the result for one catalog skill and target.
type SyncDetail struct {
	Skill  string
	Target string
	Action SyncAction
	Err    error
}

// SyncResult aggregates a catalog projection operation.
type SyncResult struct {
	Details []SyncDetail
	Errors  int
}

// Sync projects declared skills to one target. Existing divergent content is
// a conflict unless force is set; force is safe because the manifest names the
// exact source for that target. Manual entries are never resolved.
func (c *Catalog) Sync(target provider.Provider, dryRun, force bool, filter []string) (*SyncResult, error) {
	result := &SyncResult{}
	filterSet := stringSet(filter)
	for _, name := range sortedSkillNames(c.manifest.Skills) {
		if len(filterSet) > 0 && !filterSet[name] {
			continue
		}
		skill := c.manifest.Skills[name]
		if !supportsTarget(skill, target.Name()) {
			continue
		}
		if skill.Mode == modeManual {
			result.add(name, target.Name(), actionManual, fmt.Errorf("manual resolution required"))
			continue
		}
		source, err := c.readSource(name, skill, target.Name())
		if err != nil {
			result.add(name, target.Name(), actionError, err)
			continue
		}
		installed, err := target.ReadSkill(name)
		if err == nil {
			c.syncExisting(result, target, source, installed, dryRun, force)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			result.add(name, target.Name(), actionError, err)
			continue
		}
		action := actionCreated
		if dryRun {
			action = actionWouldCreate
		} else if err := writeAndVerify(target, *source); err != nil {
			result.add(name, target.Name(), actionError, err)
			continue
		}
		result.add(name, target.Name(), action, nil)
	}
	if result.Errors > 0 {
		return result, fmt.Errorf("catalog sync completed with %d unresolved/error entries", result.Errors)
	}
	return result, nil
}

func (c *Catalog) syncExisting(result *SyncResult, target provider.Provider, source, installed *provider.Skill, dryRun, force bool) {
	if syncengine.SkillsMatch(source, installed) {
		result.add(source.Name, target.Name(), actionInSync, nil)
		return
	}
	if !force {
		result.add(source.Name, target.Name(), actionConflict, fmt.Errorf("target differs; use --force only after reviewing the declared source"))
		return
	}
	action := actionUpdated
	if dryRun {
		action = actionWouldUpdate
	} else if err := writeAndVerify(target, *source); err != nil {
		result.add(source.Name, target.Name(), actionError, err)
		return
	}
	result.add(source.Name, target.Name(), action, nil)
}

func writeAndVerify(target provider.Provider, source provider.Skill) error {
	if err := target.WriteSkill(source); err != nil {
		return fmt.Errorf("write %s/%s: %w", target.Name(), source.Name, err)
	}
	written, err := target.ReadSkill(source.Name)
	if err != nil {
		return fmt.Errorf("verify %s/%s: %w", target.Name(), source.Name, err)
	}
	if !syncengine.SkillsMatch(&source, written) {
		return fmt.Errorf("verify %s/%s: content mismatch after write", target.Name(), source.Name)
	}
	return nil
}

func (r *SyncResult) add(skill, target string, action SyncAction, err error) {
	r.Details = append(r.Details, SyncDetail{Skill: skill, Target: target, Action: action, Err: err})
	if err != nil {
		r.Errors++
	}
}
