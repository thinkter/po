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

	fmt.Println("git is installed! Ready to rock.")
	// Future command handling logic will go here

	return nil
}
