package main

import (
	"errors"
	"fmt"
	"os"

	"po/internal/git"

	"github.com/charmbracelet/huh"
)

func runSave() error {
	// 1. Check status
	files, err := git.GetStatus()
	if err != nil {
		return fmt.Errorf("failed to get git status: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No changes to save.")
		return nil
	}

	// 2. Form: File Selection
	var selectedFiles []string

	// Prepare options for file selection
	// We want a special option for "." (all files)
	// Note: huh MultiSelect returns the values of the selected options.

	options := make([]huh.Option[string], 0, len(files)+1)
	options = append(options, huh.NewOption("All files (.)", "."))

	for _, f := range files {
		// Display format: [M ] path/to/file
		label := fmt.Sprintf("[%s] %s", f.Code, f.Path)
		options = append(options, huh.NewOption(label, f.Path))
	}

	// 3. Form: Commit Type
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

	// 4. Form: Commit Message
	var commitMsg string

	// Build the form
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select files to save").
				Options(options...).
				Value(&selectedFiles).
				Filterable(true),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What sort of save is it?").
				Options(typeOptions...).
				Value(&commitType),

			huh.NewInput().
				Title("Commit message").
				Placeholder("Brief description of changes").
				Value(&commitMsg),
		),
	)

	err = form.Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
		return err
	}

	if len(selectedFiles) == 0 {
		fmt.Println("No files selected. Aborting.")
		return nil
	}

	// 5. Execution

	// Handle "." selection
	stageAll := false
	for _, f := range selectedFiles {
		if f == "." {
			stageAll = true
			break
		}
	}

	fmt.Println("Staging files...")
	if stageAll {
		if err := git.StageFiles([]string{"."}); err != nil {
			return fmt.Errorf("failed to stage all files: %w", err)
		}
	} else {
		// Filter out the "." if it somehow got in mixed with others (though logic above handles it)
		// and verify files still exist in the original list to be safe, or just pass paths.
		// git.StageFiles expects paths.
		if err := git.StageFiles(selectedFiles); err != nil {
			return fmt.Errorf("failed to stage files: %w", err)
		}
	}

	fullMessage := fmt.Sprintf("%s: %s", commitType, commitMsg)
	fmt.Printf("Committing: %s\n", fullMessage)
	if err := git.Commit(fullMessage); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Println("Pushing to remote...")
	if err := git.Push(); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	fmt.Println("Success! 🚀")
	return nil
}
