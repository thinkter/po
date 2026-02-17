package main

import (
	"fmt"
	"po/internal/git"

	"github.com/charmbracelet/huh"
)

func runSync() error {
	fmt.Println("Checking for updates...")
	if err := git.Fetch(); err != nil {
		return fmt.Errorf("failed to fetch updates: %w", err)
	}

	var syncMode string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How would you like to sync?").
				Description("Choose how to bring in changes from the remote.").
				Options(
					huh.NewOption("Merge (Combine histories)", "merge"),
					huh.NewOption("Rebase (Clean, linear history)", "rebase"),
					huh.NewOption("Fast-Forward Only (Safest)", "ff-only"),
				).
				Value(&syncMode),
		),
	)

	err := form.Run()
	if err != nil {
		if err == huh.ErrUserAborted {
			fmt.Println("Aborted.")
			return nil
		}
		return err
	}

	// Explanations for the user
	switch syncMode {
	case "merge":
		fmt.Println("Merging remote changes into your branch...")
	case "rebase":
		fmt.Println("Rebasing your changes on top of remote changes...")
	case "ff-only":
		fmt.Println("Attempting fast-forward pull...")
	}

	if err := git.Pull(syncMode); err != nil {
		fmt.Println("\n❌ Sync conflict or error!")
		return err
	}

	fmt.Println("Success! Your branch is up to date. 🔄")
	return nil
}
