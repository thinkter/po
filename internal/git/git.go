package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// IsInstalled checks if the git command is available in the system PATH.
// It returns true if git is found, and false otherwise.
func IsInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// FileStatus represents the status of a file in git.
type FileStatus struct {
	Code string
	Path string
}

// GetStatus returns a list of files with their status.
func GetStatus() ([]FileStatus, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %s: %w", string(out), err)
	}

	var files []FileStatus
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if len(line) > 3 {
			code := line[:2]
			path := line[3:]
			files = append(files, FileStatus{Code: code, Path: path})
		}
	}
	return files, nil
}

// StageFiles adds the specified files to the git index.
func StageFiles(files []string) error {
	if len(files) == 0 {
		return nil
	}
	args := append([]string{"add"}, files...)
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed: %s: %w", string(output), err)
	}
	return nil
}

// Commit creates a new commit with the given message.
func Commit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit failed: %s: %w", string(output), err)
	}
	return nil
}

// GetCurrentBranch returns the name of the current branch.
func GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %s: %w", string(output), err)
	}
	return strings.TrimSpace(string(output)), nil
}

// HasUpstream checks if the current branch has an upstream tracking branch set.
func HasUpstream() bool {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	err := cmd.Run()
	return err == nil
}

// Push pushes the current branch to the remote.
// If no upstream is set, it automatically sets the upstream to origin/<current-branch>.
func Push() error {
	if !HasUpstream() {
		branch, err := GetCurrentBranch()
		if err != nil {
			return err
		}
		cmd := exec.Command("git", "push", "--set-upstream", "origin", branch)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git push failed: %s: %w", string(output), err)
		}
		return nil
	}

	cmd := exec.Command("git", "push")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed: %s: %w", string(output), err)
	}
	return nil
}

// Fetch fetches updates from the remote.
func Fetch() error {
	cmd := exec.Command("git", "fetch")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch failed: %s: %w", string(output), err)
	}
	return nil
}

// Pull pulls updates from the remote.
// mode can be "merge", "rebase", or "ff-only".
func Pull(mode string) error {
	args := []string{"pull"}
	switch mode {
	case "rebase":
		args = append(args, "--rebase")
	case "ff-only":
		args = append(args, "--ff-only")
	case "merge":
		args = append(args, "--no-rebase")
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %s: %w", string(output), err)
	}
	return nil
}
