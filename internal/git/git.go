package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
	Code    string
	Path    string
	OldPath string
	NewPath string
}

// EffectivePath returns the path that should generally be used for staging.
// For renames/copies this is the new path; otherwise it's Path.
func (f FileStatus) EffectivePath() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.Path
}

// parsePorcelainV1Line parses a single `git status --porcelain -z` entry.
// It handles:
//   - ordinary records: XY <path>\x00
//   - rename/copy records: XY <old>\x00<new>\x00
func parsePorcelainV1Line(code string, firstPath string, secondPath string) (FileStatus, error) {
	// Preserve porcelain XY exactly as-is; spaces are meaningful (e.g. " M", "M ").
	if len(code) != 2 {
		return FileStatus{}, fmt.Errorf("invalid status code %q", code)
	}

	// Decode quoted paths (if any) to avoid passing literal quotes to git add.
	unquotedFirst, err := unquoteGitPath(firstPath)
	if err != nil {
		return FileStatus{}, fmt.Errorf("failed to parse path %q: %w", firstPath, err)
	}
	unquotedSecond, err := unquoteGitPath(secondPath)
	if err != nil {
		return FileStatus{}, fmt.Errorf("failed to parse path %q: %w", secondPath, err)
	}

	// For rename/copy entries, porcelain -z emits old then new in separate NUL fields.
	if isRenameOrCopy(code) {
		return FileStatus{
			Code:    code,
			Path:    unquotedSecond, // effective path for callers expecting Path
			OldPath: unquotedFirst,
			NewPath: unquotedSecond,
		}, nil
	}

	return FileStatus{
		Code: code,
		Path: unquotedFirst,
	}, nil
}

func isRenameOrCopy(code string) bool {
	// XY two-char status: rename/copy can appear in either index or worktree position.
	// See git status porcelain docs.
	return strings.Contains(code, "R") || strings.Contains(code, "C")
}

// unquoteGitPath removes git-style quoting when present.
// For porcelain output this is usually C-style quoted strings, e.g.
// "path with spaces/file.go" or "quote\\\"name.txt".
func unquoteGitPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}

	// Fast path: not quoted
	if !(strings.HasPrefix(path, "\"") && strings.HasSuffix(path, "\"")) {
		return path, nil
	}

	decoded, err := strconv.Unquote(path)
	if err != nil {
		return "", err
	}
	return decoded, nil
}

// GetStatus returns a list of files with their status.
func GetStatus() ([]FileStatus, error) {
	// Use -z to robustly parse paths with spaces/special characters and renames.
	out, err := runGitCommand("status", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, nil
	}

	var files []FileStatus
	items := bytes.Split(out, []byte{0})

	for i := 0; i < len(items); i++ {
		entry := items[i]
		if len(entry) == 0 {
			continue
		}
		// Minimum is "XY <path>"
		if len(entry) < 4 {
			continue
		}

		code := string(entry[:2])
		// porcelain v1 guarantees one space after XY.
		firstPath := string(entry[3:])

		var secondPath string
		if isRenameOrCopy(code) {
			// Rename/copy format with -z: first entry has old path, next item is new path.
			if i+1 < len(items) {
				secondPath = string(items[i+1])
				i++
			}
		}

		fs, err := parsePorcelainV1Line(code, firstPath, secondPath)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(fs.EffectivePath()) == "" {
			continue
		}
		files = append(files, fs)
	}

	return files, nil
}

// StageFiles adds the specified files to the git index.
func StageFiles(files []string) error {
	if len(files) == 0 {
		return nil
	}

	// Revalidate currently changed paths at stage time to reduce race-window issues.
	current, err := GetStatus()
	if err != nil {
		return fmt.Errorf("failed to refresh git status before staging: %w", err)
	}
	available := make(map[string]struct{}, len(current))
	for _, f := range current {
		p := strings.TrimSpace(f.EffectivePath())
		if p != "" {
			available[p] = struct{}{}
		}
	}

	valid := make([]string, 0, len(files))
	var skipped []string
	for _, p := range files {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := available[p]; ok {
			valid = append(valid, p)
			continue
		}
		skipped = append(skipped, p)
	}

	if len(valid) == 0 {
		if len(skipped) > 0 {
			return fmt.Errorf("none of the selected paths are currently changed; they may have been modified after selection: %s", strings.Join(skipped, ", "))
		}
		return nil
	}

	args := append([]string{"add", "--"}, valid...)
	_, err = runGitCommand(args...)
	if err != nil {
		if len(skipped) > 0 {
			return fmt.Errorf("failed to stage files (also skipped stale paths: %s): %w", strings.Join(skipped, ", "), err)
		}
		return err
	}

	// Non-fatal signal to caller context through error wrapping is not ideal; we keep success
	// semantics and let caller proceed while preserving staged correctness.
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

// CommitIfStaged creates a commit only if there are staged changes.
// It returns (committed, error).
func CommitIfStaged(message string) (bool, error) {
	staged, err := HasStagedChanges()
	if err != nil {
		return false, err
	}
	if !staged {
		return false, nil
	}
	if err := Commit(message); err != nil {
		return false, err
	}
	return true, nil
}

// HasStagedChanges returns true when index differs from HEAD.
func HasStagedChanges() (bool, error) {
	_, err := runGitCommand("diff", "--cached", "--quiet")
	if err == nil {
		return false, nil
	}

	// exit code 1 means differences exist.
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return true, nil
	}
	return false, err
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

// PushCommits atomically-ish pushes all local commits once at the end of a grouped flow.
// This avoids half-pushed sequences caused by pushing between intermediate commits.
func PushCommits() error {
	return Push()
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

// IsTracked checks whether a path is currently tracked by git.
func IsTracked(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := runGitCommand("ls-files", "--error-unmatch", "--", path)
	return err == nil
}

// GetTrackedIgnoredFiles returns files that are tracked by git but now ignored
// by .gitignore or other standard excludes.
func GetTrackedIgnoredFiles() ([]string, error) {
	out, err := runGitCommand("ls-files", "-ci", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}

	raw := bytes.Split(out, []byte{0})
	files := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))

	for _, item := range raw {
		p := strings.TrimSpace(string(item))
		if p == "" {
			continue
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		files = append(files, p)
	}

	return files, nil
}

// RemoveCached removes paths from git index only (keeps files in working tree).
func RemoveCached(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	args := []string{"rm", "--cached", "--"}
	args = append(args, paths...)

	_, err := runGitCommand(args...)
	if err != nil {
		return err
	}
	return nil
}

// IsInsideWorkTree returns true when the current directory is inside a git work tree.
func IsInsideWorkTree() bool {
	_, err := runGitCommand("rev-parse", "--is-inside-work-tree")
	return err == nil
}

// IsConflictStatus returns true when the porcelain status code represents an
// unmerged (conflict) entry: UU, AA, DD, DU, UD, AU, UA.
func IsConflictStatus(code string) bool {
	if len(code) < 2 {
		return false
	}
	switch code[:2] {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return false
}
