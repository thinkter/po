package main

import (
	"fmt"
	"os"
	"po/internal/git"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if !git.IsInstalled() {
		return fmt.Errorf("error: git is not installed or not in your PATH.\nPlease install git to use po")
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "save":
			return runSave()
		case "sync":
			return runSync()
		}
	}

	fmt.Println("git is installed! Ready to rock.")
	fmt.Println("Usage:")
	fmt.Println("  po save    Stage, commit, and push changes")
	fmt.Println("  po sync    Fetch and pull updates from remote")

	return nil
}
