package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"po/internal/git"
	"po/internal/workflow"

	"github.com/charmbracelet/huh"
)

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

	// Build a deduped list of status paths for safe normalization/validation.
	statusPathSet := make(map[string]struct{}, len(files))
	statusPaths := make([]string, 0, len(files))
	for _, f := range files {
		p := strings.TrimSpace(f.Path)
		if p == "" {
			continue
		}
		if _, exists := statusPathSet[p]; !exists {
			statusPathSet[p] = struct{}{}
			statusPaths = append(statusPaths, p)
		}
	}

	if len(statusPaths) == 0 {
		fmt.Println("No valid changed paths found.")
		return nil
	}

	// 2) File selection form (only file selection in first step)
	var selectedRaw []string
	fileOptions := make([]huh.Option[string], 0, len(statusPaths)+1)
	fileOptions = append(fileOptions, huh.NewOption("All changed files", "__ALL__"))

	// Display each changed path once.
	displayed := make(map[string]struct{}, len(statusPaths))
	for _, f := range files {
		p := strings.TrimSpace(f.Path)
		if p == "" {
			continue
		}
		if _, seen := displayed[p]; seen {
			continue
		}
		displayed[p] = struct{}{}
		// Keep label informative, value is the path.
		label := fmt.Sprintf("[%s] %s", strings.TrimSpace(f.Code), p)
		fileOptions = append(fileOptions, huh.NewOption(label, p))
	}

	selectForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select files to save").
				Description("Choose one or many files. Use space to toggle selections.").
				Options(fileOptions...).
				Value(&selectedRaw).
				Filterable(true),
		),
	)

	if err := selectForm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
		return err
	}

	if len(selectedRaw) == 0 {
		fmt.Println("No files selected. Aborting.")
		return nil
	}

	// 3) Normalize selected file list
	stageAll := false
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
		huh.NewOption("perf: Performance improvement", "perf"),
		huh.NewOption("test: Add or modify tests", "test"),
		huh.NewOption("docs: Documentation only", "docs"),
		huh.NewOption("style: Formatting, lint, no logic", "style"),
		huh.NewOption("build: Build system, deps", "build"),
		huh.NewOption("ci: CI/CD config", "ci"),
		huh.NewOption("chore: Maintenance, scripts", "chore"),
		huh.NewOption("revert: Revert a commit", "revert"),
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
			os.Exit(0)
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

	selectedFiles := make([]string, 0, len(selectedSet))
	for p := range selectedSet {
		selectedFiles = append(selectedFiles, p)
	}

	if err := workflow.Save(selectedFiles, fullMessage); err != nil {
		return err
	}

	return nil
}
