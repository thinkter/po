package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"po/internal/git"

	"github.com/charmbracelet/huh"
)

// errAborted is returned when the user presses Ctrl-C / Esc during a form.
// Returning an error instead of calling os.Exit(0) preserves defer semantics.
var errAborted = errors.New("user aborted")

func runSave() error {
	// 1) Read git status
	files, err := git.GetStatus()
	if err != nil {
		return fmt.Errorf("failed to get git status: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No changes to save.")
		return nil
	}

	// Separate conflicted files and warn the user about them.
	var conflicted []string
	var clean []git.FileStatus
	for _, f := range files {
		if git.IsConflictStatus(f.Code) {
			conflicted = append(conflicted, f.EffectivePath())
		} else {
			clean = append(clean, f)
		}
	}

	if len(conflicted) > 0 {
		fmt.Println("⚠ The following files have merge conflicts and will be excluded:")
		for _, p := range conflicted {
			fmt.Printf("  • %s\n", p)
		}
		fmt.Println("Resolve conflicts before saving these files.")
		fmt.Println()
	}

	if len(clean) == 0 {
		fmt.Println("All changed files have merge conflicts. Resolve them first.")
		return nil
	}

	// Build a deduped list of status paths for safe normalization/validation.
	// Uses EffectivePath() so renames resolve to the new path and quoted paths are already unquoted.
	statusPathSet := make(map[string]struct{}, len(clean))
	statusPaths := make([]string, 0, len(clean))
	statusSnapshot := make(map[string]string, len(clean))
	for _, f := range clean {
		p := strings.TrimSpace(f.EffectivePath())
		if p == "" {
			continue
		}
		statusSnapshot[p] = normalizeStatusCode(f.Code)
		if _, exists := statusPathSet[p]; !exists {
			statusPathSet[p] = struct{}{}
			statusPaths = append(statusPaths, p)
		}
	}

	if len(statusPaths) == 0 {
		fmt.Println("No valid changed paths found.")
		return nil
	}

	// 2) File selection form
	var selectedRaw []string
	fileOptions := make([]huh.Option[string], 0, len(statusPaths)+1)
	fileOptions = append(fileOptions, huh.NewOption("All changed files", "__ALL__"))

	// Display each changed path once.
	displayed := make(map[string]struct{}, len(statusPaths))
	for _, f := range clean {
		p := strings.TrimSpace(f.EffectivePath())
		if p == "" {
			continue
		}
		if _, seen := displayed[p]; seen {
			continue
		}
		displayed[p] = struct{}{}

		// Show rename old→new in the label so users understand what happened.
		label := fmt.Sprintf("[%s] %s", strings.TrimSpace(f.Code), p)
		if f.OldPath != "" && f.NewPath != "" {
			label = fmt.Sprintf("[%s] %s → %s", strings.TrimSpace(f.Code), f.OldPath, f.NewPath)
		}
		fileOptions = append(fileOptions, huh.NewOption(label, p))
	}

	selectForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select files to save").
				// Description("Choose files to save.").
				Options(fileOptions...).
				Value(&selectedRaw).
				Filterable(true),
		),
	)

	if err := selectForm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Println("Aborted.")
			return nil
		}
		return err
	}

	if len(selectedRaw) == 0 {
		fmt.Println("No files selected. Aborting.")
		return nil
	}

	// 3) Normalize selected file list
	stageAll := false
	hasIndividual := false
	normalizedSet := make(map[string]struct{}, len(selectedRaw))
	normalized := make([]string, 0, len(selectedRaw))

	for _, item := range selectedRaw {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if v == "__ALL__" {
			stageAll = true
			continue
		}
		hasIndividual = true
		// Only allow paths that are currently reported by git status.
		if _, ok := statusPathSet[v]; !ok {
			continue
		}
		if _, seen := normalizedSet[v]; seen {
			continue
		}
		normalizedSet[v] = struct{}{}
		normalized = append(normalized, v)
	}

	// Warn when __ALL__ is selected alongside individual files.
	if stageAll && hasIndividual {
		fmt.Println("Note: 'All changed files' was selected — including all files.")
	}

	if !stageAll && len(normalized) == 0 {
		fmt.Println("No valid files selected. Aborting.")
		return nil
	}

	// 4) Commit metadata form (after file selection)
	var commitType string
	typeOptions := []huh.Option[string]{
		huh.NewOption("feat: New user-facing capability", "feat"),
		huh.NewOption("fix: Bug fix", "fix"),
		huh.NewOption("refactor: Code change without behavior change", "refactor"),
	}

	var commitMsg string
	commitForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What sort of save is it?").
				Options(typeOptions...).
				Value(&commitType),

			huh.NewInput().
				Title("Commit message").
				Placeholder("Brief description of changes").
				Value(&commitMsg).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("commit message cannot be empty")
					}
					return nil
				}),
		),
	)

	if err := commitForm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Println("Aborted.")
			return nil
		}
		return err
	}

	fullMessage := fmt.Sprintf("%s: %s", commitType, strings.TrimSpace(commitMsg))

	// Build final selected set for easy membership checks.
	selectedSet := make(map[string]struct{}, len(statusPaths))
	if stageAll {
		for _, p := range statusPaths {
			selectedSet[p] = struct{}{}
		}
	} else {
		for _, p := range normalized {
			selectedSet[p] = struct{}{}
		}
	}

	// Detect ALL .gitignore files in the selected set (not just one).
	var gitignorePaths []string
	for p := range selectedSet {
		if p == ".gitignore" || strings.HasSuffix(p, "/.gitignore") {
			gitignorePaths = append(gitignorePaths, p)
		}
	}

	// Detect selection drift before staging, so users can decide how to proceed.
	drift, err := detectSelectionDrift(selectedSet, statusSnapshot)
	if err != nil {
		return fmt.Errorf("failed to re-check file status before staging: %w", err)
	}
	if drift.hasChanges() {
		fmt.Println("Some selected files changed after you made your selection:")
		for _, p := range drift.missing {
			fmt.Printf("  • %s (no longer changed or missing)\n", p)
		}
		for _, entry := range drift.changed {
			fmt.Printf("  • %s\n", entry)
		}
		fmt.Println()

		var driftChoice string
		driftForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Selection drift detected. What do you want to do?").
					Options(
						huh.NewOption("Continue with currently available files", "continue"),
						huh.NewOption("Cancel and re-run po save", "rerun"),
						huh.NewOption("Abort", "abort"),
					).
					Value(&driftChoice),
			),
		)

		if err := driftForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("Aborted.")
				return nil
			}
			return err
		}

		switch driftChoice {
		case "continue":
			for _, p := range drift.missing {
				delete(selectedSet, p)
			}
		case "rerun":
			fmt.Println("Canceled. Re-run `po save` to make a fresh selection.")
			return nil
		default:
			fmt.Println("Aborted.")
			return nil
		}
	}

	// 5) If .gitignore files are part of this save, commit them first.
	//    All commits are made locally first; pushing is deferred to the end
	//    so we never end up in a half-pushed state.
	for _, gip := range gitignorePaths {
		fmt.Printf("Prioritizing %s in a separate commit...\n", gip)
		staged, skipped, err := git.StageFilesWithReport([]string{gip})
		if err != nil {
			return fmt.Errorf("failed to stage %s: %w", gip, err)
		}
		if len(skipped) > 0 {
			fmt.Printf("  Skipped %s (it changed after selection).\n", gip)
			delete(selectedSet, gip)
			continue
		}
		if len(staged) == 0 {
			delete(selectedSet, gip)
			continue
		}
		committed, err := git.CommitIfStaged("chore(gitignore): update ignore rules")
		if err != nil {
			return fmt.Errorf("failed to commit %s: %w", gip, err)
		}
		if !committed {
			fmt.Printf("  (no changes to commit for %s, skipping)\n", gip)
		}

		// Don't include this .gitignore in the final general commit.
		delete(selectedSet, gip)
	}

	// After ignore-rule updates (including .git/info/exclude), optionally untrack
	// files that are now ignored. This is not limited to .gitignore paths.
	ignoredTracked, err := git.GetTrackedIgnoredFiles()
	if err != nil {
		return fmt.Errorf("failed to detect tracked ignored files: %w", err)
	}
	if len(ignoredTracked) > 0 {
		reason := "current ignore rules"
		if len(gitignorePaths) > 0 {
			reason = "updated .gitignore rules"
		}
		fmt.Printf("Found %d tracked file(s) matched by %s.\n", len(ignoredTracked), reason)
		for _, p := range ignoredTracked {
			fmt.Printf("  • %s\n", p)
		}
		fmt.Println()

		var cleanupChoice string
		cleanupForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Untrack these ignored files now?").
					Options(
						huh.NewOption("Yes, untrack them in a separate commit", "yes"),
						huh.NewOption("No, keep them tracked", "no"),
					).
					Value(&cleanupChoice),
			),
		)

		if err := cleanupForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("Aborted.")
				return nil
			}
			return err
		}

		if cleanupChoice == "yes" {
			fmt.Printf("Untracking %d ignored file(s) from repository...\n", len(ignoredTracked))
			if err := git.RemoveCached(ignoredTracked); err != nil {
				return fmt.Errorf("failed to untrack ignored files: %w", err)
			}
			committed, err := git.CommitIfStaged("chore(ignore): untrack ignored files")
			if err != nil {
				return fmt.Errorf("failed to commit ignored-file cleanup: %w", err)
			}
			if !committed {
				fmt.Println("  (no tracked-ignored files needed cleanup)")
			}
		} else {
			fmt.Println("Keeping currently tracked ignored files unchanged.")
		}
	}

	// 6) Stage and commit remaining selected files.
	remaining := make([]string, 0, len(selectedSet))
	for _, p := range statusPaths {
		if _, ok := selectedSet[p]; ok {
			remaining = append(remaining, p)
		}
	}

	if len(remaining) > 0 {
		fmt.Println("Staging remaining selected files...")
		staged, skipped, err := git.StageFilesWithReport(remaining)
		if err != nil {
			return fmt.Errorf("failed to stage selected files: %w", err)
		}
		if len(skipped) > 0 {
			fmt.Println("Some selected files were not staged because they changed after selection:")
			for _, p := range skipped {
				fmt.Printf("  • %s\n", p)
			}
		}
		if len(staged) == 0 {
			fmt.Println("No selected files could be staged from your current selection.")
			return nil
		}

		fmt.Printf("Committing: %s\n", fullMessage)
		committed, err := git.CommitIfStaged(fullMessage)
		if err != nil {
			return fmt.Errorf("failed to commit: %w", err)
		}
		if !committed {
			fmt.Println("No staged changes to commit (files may have been modified after selection).")
		}
	} else if len(gitignorePaths) == 0 {
		fmt.Println("No remaining selected files to save.")
		return nil
	}

	// 7) Single push at the end — all commits go out together.
	fmt.Println("Pushing to remote...")
	if err := git.Push(); err != nil {
		return fmt.Errorf("failed to push (your commits are saved locally — run 'git push' to retry): %w", err)
	}

	fmt.Println("Success!")
	return nil
}

type selectionDrift struct {
	missing []string
	changed []string
}

func (d selectionDrift) hasChanges() bool {
	return len(d.missing) > 0 || len(d.changed) > 0
}

func detectSelectionDrift(selectedSet map[string]struct{}, snapshot map[string]string) (selectionDrift, error) {
	current, err := git.GetStatus()
	if err != nil {
		return selectionDrift{}, err
	}

	currentByPath := make(map[string]string, len(current))
	for _, f := range current {
		p := strings.TrimSpace(f.EffectivePath())
		if p == "" {
			continue
		}
		currentByPath[p] = normalizeStatusCode(f.Code)
	}

	drift := selectionDrift{
		missing: []string{},
		changed: []string{},
	}

	for p := range selectedSet {
		before, hadBefore := snapshot[p]
		now, hasNow := currentByPath[p]

		if !hasNow {
			drift.missing = append(drift.missing, p)
			continue
		}
		if hadBefore && before != now {
			drift.changed = append(drift.changed, fmt.Sprintf("%s (%s -> %s)", p, compactStatusCode(before), compactStatusCode(now)))
		}
	}

	sort.Strings(drift.missing)
	sort.Strings(drift.changed)

	return drift, nil
}

func compactStatusCode(code string) string {
	norm := normalizeStatusCode(code)
	if norm == "??" {
		return norm
	}
	c := strings.ReplaceAll(norm, " ", "")
	if c == "" {
		return "--"
	}
	return c
}
