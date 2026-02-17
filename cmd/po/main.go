package main

import (
	"fmt"
	"os"
	"po/internal/git"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

	return runDefaultStatus()
}

func runDefaultStatus() error {
	branch, upstream, err := getBranchAndUpstream()
	if err != nil {
		return err
	}

	files, err := git.GetStatus()
	if err != nil {
		return err
	}

	// Group into only:
	// - saved   => staged
	// - unsaved => unstaged + untracked
	type statusGroup struct {
		saved   []string
		unsaved []string
	}

	groups := statusGroup{}
	savedSet := map[string]struct{}{}
	unsavedSet := map[string]struct{}{}

	for _, f := range files {
		code := normalizeStatusCode(f.Code)
		path := strings.TrimSpace(f.Path)
		if path == "" {
			continue
		}

		// Untracked is always unsaved.
		if strings.HasPrefix(code, "??") {
			if _, exists := unsavedSet[path]; !exists {
				unsavedSet[path] = struct{}{}
				groups.unsaved = append(groups.unsaved, path)
			}
			continue
		}

		// X column => staged => saved
		if len(code) >= 1 && code[0] != ' ' && code[0] != '?' {
			if _, exists := savedSet[path]; !exists {
				savedSet[path] = struct{}{}
				groups.saved = append(groups.saved, path)
			}
		}

		// Y column => unstaged => unsaved
		if len(code) >= 2 && code[1] != ' ' && code[1] != '?' {
			if _, exists := unsavedSet[path]; !exists {
				unsavedSet[path] = struct{}{}
				groups.unsaved = append(groups.unsaved, path)
			}
		}
	}

	totalChanges := len(files)
	savedCount := len(groups.saved)
	unsavedCount := len(groups.unsaved)

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("62")).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	okStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	warnStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))

	savedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	unsavedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

	sectionHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	fileBulletStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	// Header
	fmt.Println(titleStyle.Render("po status"))

	// Summary
	fmt.Println(lipgloss.JoinHorizontal(
		lipgloss.Top,
		labelStyle.Render("Branch: "),
		valueStyle.Render(branch),
	))

	if upstream != "" {
		fmt.Println(lipgloss.JoinHorizontal(
			lipgloss.Top,
			labelStyle.Render("Tracking: "),
			valueStyle.Render(upstream),
		))
	} else {
		fmt.Println(lipgloss.JoinHorizontal(
			lipgloss.Top,
			labelStyle.Render("Tracking: "),
			warnStyle.Render("(no upstream set)"),
		))
	}

	if totalChanges == 0 {
		fmt.Println(okStyle.Render("Status: Clean working tree."))
	} else {
		fmt.Println(warnStyle.Render(fmt.Sprintf("Status: %d changed file(s)", totalChanges)))
		fmt.Println("  " + savedStyle.Render(fmt.Sprintf("Saved: %d", savedCount)))
		fmt.Println("  " + unsavedStyle.Render(fmt.Sprintf("Unsaved: %d", unsavedCount)))
	}

	// File lists
	if totalChanges > 0 {
		fmt.Println()

		renderFileSection := func(title string, items []string, titleS lipgloss.Style) {
			if len(items) == 0 {
				return
			}
			fmt.Println(sectionHeaderStyle.Render("▸ " + titleS.Render(title)))
			for _, p := range items {
				fmt.Println("  " + fileBulletStyle.Render("•") + " " + valueStyle.Render(p))
			}
			fmt.Println()
		}

		renderFileSection("Unsaved files", groups.unsaved, unsavedStyle)
		renderFileSection("Saved files", groups.saved, savedStyle)
	}

	// Hints
	fmt.Println(sectionHeaderStyle.Render("Hints"))
	fmt.Println("  " + infoStyle.Render("• Use `po save` to save and push your changes."))
	fmt.Println("  " + infoStyle.Render("• Use `po sync` to fetch and pull updates from remote."))

	return nil
}

func getBranchAndUpstream() (branch string, upstream string, err error) {
	branch, err = git.GetCurrentBranch()
	if err != nil {
		return "", "", fmt.Errorf("failed to determine current branch: %w", err)
	}

	if git.HasUpstream() {
		upstream = "upstream configured"
	}

	return branch, upstream, nil
}

func normalizeStatusCode(code string) string {
	switch {
	case len(code) >= 2:
		return code[:2]
	case len(code) == 1:
		return code + " "
	default:
		return "  "
	}
}
