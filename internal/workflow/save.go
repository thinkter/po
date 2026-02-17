package workflow

import (
	"fmt"
	"strings"

	"po/internal/git"
)

// Save handles the logic of saving files:
// 1. Checks if .gitignore is in the selected files.
// 2. If so, commits and pushes .gitignore first.
// 3. Checks for tracked-but-ignored files and removes them.
// 4. Commits and pushes the remaining files.
func Save(selectedFiles []string, commitMessage string) error {
	if len(selectedFiles) == 0 {
		return nil
	}

	selectedSet := make(map[string]struct{}, len(selectedFiles))
	for _, f := range selectedFiles {
		selectedSet[f] = struct{}{}
	}

	// Detect whether selected changes include .gitignore.
	gitignoreCandidates := []string{".gitignore"}
	// Also check for nested .gitignores if they are in the selected set
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
	// efficient fallback if not found in tracked list (maybe it's a new file)
	if gitignorePath == "" {
		for _, candidate := range gitignoreCandidates {
			if _, ok := selectedSet[candidate]; ok {
				gitignorePath = candidate
				break
			}
		}
	}

	// If .gitignore is part of this save, commit and push it first.
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

	// Stage, commit, push remaining selected files.
	remaining := make([]string, 0, len(selectedSet))
	for p := range selectedSet {
		remaining = append(remaining, p)
	}

	if len(remaining) == 0 {
		if gitignorePath != "" {
			fmt.Println("No remaining selected files to save after gitignore flow.")
			fmt.Println("Success!")
			return nil
		}
		// If we had no files to start with, we shouldn't have reached here theoretically, 
		// but safeguards are good.
		return nil
	}

	fmt.Println("Staging remaining selected files...")
	if err := git.StageFiles(remaining); err != nil {
		return fmt.Errorf("failed to stage selected files: %w", err)
	}

	fmt.Printf("Committing: %s\n", commitMessage)
	if err := git.Commit(commitMessage); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Println("Pushing to remote...")
	if err := git.Push(); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	fmt.Println("Success!")
	return nil
}
