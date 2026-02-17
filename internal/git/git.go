package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runGitCommand runs a git command in the current working directory
// (the same repository where `po` was invoked).
func runGitCommand(args ...string) ([]byte, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = wd

	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("git %s failed: %s: %w", strings.Join(args, " "), string(output), err)
	}

	return output, nil
}

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
	out, err := runGitCommand("status", "--porcelain")
	if err != nil {
		return nil, err
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
	_, err := runGitCommand(args...)
	if err != nil {
		return err
	}
	return nil
}

// Commit creates a new commit with the given message.
func Commit(message string) error {
	_, err := runGitCommand("commit", "-m", message)
	if err != nil {
		return err
	}
	return nil
}

// GetCurrentBranch returns the name of the current branch.
func GetCurrentBranch() (string, error) {
	output, err := runGitCommand("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// HasUpstream checks if the current branch has an upstream tracking branch set.
func HasUpstream() bool {
	_, err := runGitCommand("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
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
		_, err = runGitCommand("push", "--set-upstream", "origin", branch)
		if err != nil {
			return err
		}
		return nil
	}

	_, err := runGitCommand("push")
	if err != nil {
		return err
	}
	return nil
}

// Fetch fetches updates from the remote.
func Fetch() error {
	_, err := runGitCommand("fetch")
	if err != nil {
		return err
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

	_, err := runGitCommand(args...)
	if err != nil {
		return err
	}
	return nil
}
