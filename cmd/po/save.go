package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"po/internal/git"

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
	statusPathSet := make(map[string]struct{}, len(files)) //still don't really understand why does this builtin function needs to exist but it just makes this map accessible
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
		// huh.NewOption("perf: Performance improvement", "perf"),
		// huh.NewOption("test: Add or modify tests", "test"),
		// huh.NewOption("docs: Documentation only", "docs"),
		// huh.NewOption("style: Formatting, lint, no logic", "style"),
		// huh.NewOption("build: Build system, deps", "build"),
		// huh.NewOption("ci: CI/CD config", "ci"),
		// huh.NewOption("chore: Maintenance, scripts", "chore"),
		// huh.NewOption("revert: Revert a commit", "revert"),
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

	// Detect whether selected changes include .gitignore.
	gitignoreCandidates := []string{".gitignore"}
	for p := range selectedSet {
		if strings.HasSuffix(p, "/.gitignore") {
			gitignoreCandidates = append(gitignoreCandidates, p)
		}
	}
	gitignorePath := ""
	for _, candidate := range gitignoreCandidates {
		if _, ok := selectedSet[candidate]; ok && git.IsTracked(candidate) {
			gitignorePath = candidate
			break
		}
	}
	if gitignorePath == "" {
		for _, candidate := range gitignoreCandidates {
			if _, ok := selectedSet[candidate]; ok {
				gitignorePath = candidate
				break
			}
		}
	}

	// 5) If .gitignore is part of this save, commit and push it first.
	if gitignorePath != "" {
		fmt.Printf("Prioritizing %s in a separate commit...\n", gitignorePath)
		if err := git.StageFiles([]string{gitignorePath}); err != nil {
			return fmt.Errorf("failed to stage %s: %w", gitignorePath, err)
		}
		if err := git.Commit("chore(gitignore): update ignore rules"); err != nil {
			return fmt.Errorf("failed to commit %s: %w", gitignorePath, err)
		}
		fmt.Println("Pushing .gitignore commit...")
		if err := git.Push(); err != nil {
			return fmt.Errorf("failed to push %s commit: %w", gitignorePath, err)
		}

		// After .gitignore update lands, remove now-ignored tracked files.
		fmt.Println("Checking for tracked files now ignored by updated .gitignore...")
		ignoredTracked, err := git.GetTrackedIgnoredFiles()
		if err != nil {
			return fmt.Errorf("failed to detect tracked ignored files: %w", err)
		}

		if len(ignoredTracked) > 0 {
			fmt.Printf("Untracking %d ignored file(s) from repository...\n", len(ignoredTracked))
			if err := git.RemoveCached(ignoredTracked); err != nil {
				return fmt.Errorf("failed to untrack ignored files: %w", err)
			}
			if err := git.Commit("chore(gitignore): untrack ignored files"); err != nil {
				return fmt.Errorf("failed to commit ignored-file cleanup: %w", err)
			}
			fmt.Println("Pushing ignored-file cleanup commit...")
			if err := git.Push(); err != nil {
				return fmt.Errorf("failed to push ignored-file cleanup commit: %w", err)
			}
		} else {
			fmt.Println("No tracked files are currently matched by ignore rules.")
		}

		// Don't include .gitignore in the final general commit.
		delete(selectedSet, gitignorePath)
	}

	// 6) Stage, commit, push remaining selected files.
	remaining := make([]string, 0, len(selectedSet))
	for _, p := range statusPaths {
		if _, ok := selectedSet[p]; ok {
			remaining = append(remaining, p)
		}
	}

	if len(remaining) == 0 {
		fmt.Println("No remaining selected files to save after gitignore flow.")
		fmt.Println("Success!")
		return nil
	}

	fmt.Println("Staging remaining selected files...")
	if err := git.StageFiles(remaining); err != nil {
		return fmt.Errorf("failed to stage selected files: %w", err)
	}

	fmt.Printf("Committing: %s\n", fullMessage)
	if err := git.Commit(fullMessage); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Println("Pushing to remote...")
	if err := git.Push(); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	fmt.Println("Success!")
	return nil
}
