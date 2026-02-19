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

	if !git.IsInsideWorkTree() {
		return fmt.Errorf("error: not inside a git repository.\nRun 'git init' first or navigate to your project folder")
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

// fileGroup holds a labelled, styled list of paths for display.
type fileGroup struct {
	label string
	style lipgloss.Style
	items []statusItem
}

// statusItem keeps display text paired with the porcelain code.
type statusItem struct {
	code    string
	display string
}

func classifyStatus(code string) string {
	if len(code) < 2 {
		return "modified"
	}
	if git.IsConflictStatus(code) {
		return "conflicted"
	}
	xy := code[:2]
	switch {
	case xy == "??":
		return "new"
	case strings.ContainsAny(xy, "RC"):
		return "renamed"
	case strings.ContainsAny(xy, "D"):
		return "deleted"
	case strings.ContainsAny(xy, "A"):
		return "new"
	default:
		return "modified"
	}
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
	sectionHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	fileBulletStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	modifiedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	newStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	deletedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	renamedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	conflictStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Background(lipgloss.Color("52"))

	// Categorise files into groups.
	groups := map[string]*fileGroup{
		"modified":   {label: "Modified", style: modifiedStyle},
		"new":        {label: "New", style: newStyle},
		"deleted":    {label: "Deleted", style: deletedStyle},
		"renamed":    {label: "Renamed", style: renamedStyle},
		"conflicted": {label: "Conflicted", style: conflictStyle},
	}
	// Maintain display order.
	groupOrder := []string{"conflicted", "modified", "new", "deleted", "renamed"}

	seen := map[string]struct{}{}
	totalCount := 0

	for _, f := range files {
		p := strings.TrimSpace(f.EffectivePath())
		if p == "" {
			continue
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		totalCount++

		kind := classifyStatus(f.Code)

		display := p
		if f.OldPath != "" && f.NewPath != "" {
			display = f.OldPath + " → " + f.NewPath
		}

		groups[kind].items = append(groups[kind].items, statusItem{
			code:    formatStatusCodeDisplay(f.Code),
			display: display,
		})
	}

	// Render header
	fmt.Println(titleStyle.Render("po status"))

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

	if totalCount == 0 {
		fmt.Println(okStyle.Render("Status: Clean working tree."))
	} else {
		fmt.Println(warnStyle.Render(fmt.Sprintf("Status: %d unsaved file(s)", totalCount)))
		fmt.Println()

		for _, kind := range groupOrder {
			g := groups[kind]
			if len(g.items) == 0 {
				continue
			}
			fmt.Println(sectionHeaderStyle.Render("▸ ") + g.style.Render(fmt.Sprintf("%s (%d)", g.label, len(g.items))))
			for _, item := range g.items {
				fmt.Println("  " + fileBulletStyle.Render("•") + " " + fileBulletStyle.Render("["+item.code+"]") + " " + valueStyle.Render(item.display))
			}
		}
	}

	fmt.Println()
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

func formatStatusCodeDisplay(code string) string {
	norm := normalizeStatusCode(code)
	if norm == "??" {
		return norm
	}

	compact := strings.ReplaceAll(norm, " ", "")
	if compact == "" {
		return "--"
	}
	return compact
}
