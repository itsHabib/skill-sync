package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/itsHabib/skill-sync/internal/catalog"
	"github.com/itsHabib/skill-sync/internal/provider"
	"github.com/itsHabib/skill-sync/internal/sync"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync drift between providers",
	Long: `Compares skills in your source provider against all targets and reports
which skills are in sync, modified, missing, or extra.

Exits with code 1 if any drift is detected -- useful for CI checks.`,
	Example: `  # Check drift across all targets
  skill-sync status

  # Check only specific skills
  skill-sync status --skill deploy --skill review

  # Use inline providers (no config file)
  skill-sync status --source claude --targets copilot,gemini

  # Check a policy catalog against Claude and Codex as JSON
  skill-sync status --source-dir . --manifest catalog.yaml --targets claude,codex --json`,
	RunE: runStatus,
}

var statusSkills []string
var statusJSON bool

func init() {
	statusCmd.Flags().StringSliceVar(&statusSkills, "skill", nil, "check only named skills (repeatable)")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "write a stable machine-readable drift report")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, _ []string) error {
	source, targets, err := resolveProviders(Cfg)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	engine := sync.NewDiffEngine(source, targets)
	if manifestPath != "" {
		catalogPolicy, err := catalog.Load(Cfg.SourceDir, manifestPath)
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		report, err := catalogPolicy.Status(targets)
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		if statusJSON {
			return writeStatusJSON(cmd.OutOrStdout(), report, statusSkills)
		}
		return writeStatus(cmd.OutOrStdout(), report, statusSkills)
	}
	if statusJSON {
		return doStatusJSON(cmd.OutOrStdout(), engine, statusSkills)
	}
	return doStatus(cmd.OutOrStdout(), engine, statusSkills)
}

func doStatus(w io.Writer, engine *sync.DiffEngine, skillFilter []string) error {
	report, err := engine.Status()
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	return writeStatus(w, report, skillFilter)
}

func writeStatus(w io.Writer, report *sync.DriftReport, skillFilter []string) error {
	filterSet := make(map[string]bool, len(skillFilter))
	for _, name := range skillFilter {
		filterSet[name] = true
	}

	hasDrift := false
	for _, targetName := range sortedTargetNames(report) {
		drifts := sortedDrifts(report.Results[targetName])
		fmt.Fprintf(w, "Target: %s\n", targetName)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SKILL\tSTATUS")

		for _, d := range drifts {
			if len(filterSet) > 0 && !filterSet[d.SkillName] {
				continue
			}
			symbol := statusSymbol(d.Status)
			fmt.Fprintf(tw, "%s\t%s\n", d.SkillName, symbol)
			if isDriftStatus(d.Status) {
				hasDrift = true
			}
		}

		tw.Flush()
		fmt.Fprintln(w)
	}

	if hasDrift {
		return fmt.Errorf("drift detected")
	}

	return nil
}

type jsonStatusReport struct {
	Drift   bool               `json:"drift"`
	Targets []jsonStatusTarget `json:"targets"`
}

type jsonStatusTarget struct {
	Name   string            `json:"name"`
	Skills []jsonStatusSkill `json:"skills"`
}

type jsonStatusSkill struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func doStatusJSON(w io.Writer, engine *sync.DiffEngine, skillFilter []string) error {
	report, err := engine.Status()
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	return writeStatusJSON(w, report, skillFilter)
}

func writeStatusJSON(w io.Writer, report *sync.DriftReport, skillFilter []string) error {
	filterSet := make(map[string]bool, len(skillFilter))
	for _, name := range skillFilter {
		filterSet[name] = true
	}

	out := jsonStatusReport{}
	for _, targetName := range sortedTargetNames(report) {
		target := jsonStatusTarget{Name: targetName, Skills: []jsonStatusSkill{}}
		for _, drift := range sortedDrifts(report.Results[targetName]) {
			if len(filterSet) > 0 && !filterSet[drift.SkillName] {
				continue
			}
			target.Skills = append(target.Skills, jsonStatusSkill{
				Name:   drift.SkillName,
				Status: drift.Status.String(),
			})
			if isDriftStatus(drift.Status) {
				out.Drift = true
			}
		}
		out.Targets = append(out.Targets, target)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		return fmt.Errorf("status: encode JSON: %w", err)
	}
	if out.Drift {
		return fmt.Errorf("drift detected")
	}
	return nil
}

func sortedTargetNames(report *sync.DriftReport) []string {
	names := make([]string, 0, len(report.Results))
	for name := range report.Results {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedDrifts(drifts []sync.SkillDrift) []sync.SkillDrift {
	sorted := append([]sync.SkillDrift(nil), drifts...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].SkillName < sorted[j].SkillName
	})
	return sorted
}

func statusSymbol(s provider.SkillStatus) string {
	switch s {
	case provider.InSync:
		return "[ok] in-sync"
	case provider.Modified:
		return "[!] modified"
	case provider.MissingInTarget:
		return "[-] missing"
	case provider.ExtraInTarget:
		return "[+] extra"
	case provider.Manual:
		return "[?] manual"
	case provider.Unmanaged:
		return "[~] unmanaged"
	default:
		return "[?] unknown"
	}
}

func isDriftStatus(status provider.SkillStatus) bool {
	return status != provider.InSync && status != provider.Unmanaged
}
