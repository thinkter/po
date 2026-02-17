package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a temporary directory, initializes a git repo,
// and returns the path and a cleanup function.
// It also changes the current working directory to the temp repo,
// and restores it on cleanup, because our git package uses os.Getwd().
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	// 1. Create temp dir
	tmpDir := t.TempDir()

	// 2. Save current WD
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current wd: %v", err)
	}

	// 3. Change WD to temp dir
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}

	// 4. Init git repo
	cmd := exec.Command("git", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %s", out)
	}

	// Configure git user for commits
	exec.Command("git", "config", "user.email", "you@example.com").Run()
	exec.Command("git", "config", "user.name", "Your Name").Run()

	cleanup := func() {
		os.Chdir(originalWd)
	}

	return tmpDir, cleanup
}

func TestGetStatus(t *testing.T) {
	_, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create a file
	if err := os.WriteFile("foo.txt", []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// git status should show it as untracked (??)
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if len(status) != 1 {
		t.Fatalf("expected 1 status item, got %d", len(status))
	}
	if status[0].Path != "foo.txt" {
		t.Errorf("expected path foo.txt, got %s", status[0].Path)
	}
	if status[0].Code != "??" {
		t.Errorf("expected code ??, got %s", status[0].Code)
	}

	// Add the file
	if err := StageFiles([]string{"foo.txt"}); err != nil {
		t.Fatalf("StageFiles failed: %v", err)
	}

	status, err = GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(status) != 1 {
		t.Fatalf("expected 1 status item, got %d", len(status))
	}
	if status[0].Code != "A " {
		t.Errorf("expected code 'A ', got '%s'", status[0].Code)
	}
}

func TestIsTracked(t *testing.T) {
	_, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file
	if err := os.WriteFile("tracked.txt", []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked.txt").Run()

	// Create an untracked file
	if err := os.WriteFile("untracked.txt", []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if !IsTracked("tracked.txt") {
		t.Error("expected tracked.txt to be tracked")
	}
	if IsTracked("untracked.txt") {
		t.Error("expected untracked.txt to be untracked")
	}
}

func TestGetTrackedIgnoredFiles(t *testing.T) {
	_, cleanup := setupTestRepo(t)
	defer cleanup()

	// 1. Create a file and track it
	fileName := "config.secret"
	if err := os.WriteFile(fileName, []byte("shhh"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	exec.Command("git", "add", fileName).Run()
	exec.Command("git", "commit", "-m", "add secret").Run()

	// 2. Add it to .gitignore
	if err := os.WriteFile(".gitignore", []byte("*.secret\n"), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	// 3. Check if it's detected as tracked-but-ignored
	// Note: We need to make sure .gitignore is effective.
	// IsTracked should still be true because it's in the index.
	if !IsTracked(fileName) {
		t.Error("expected file to still be tracked")
	}

	ignored, err := GetTrackedIgnoredFiles()
	if err != nil {
		t.Fatalf("GetTrackedIgnoredFiles failed: %v", err)
	}

	found := false
	for _, f := range ignored {
		if f == fileName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected %s to be in ignored list, got %v", fileName, ignored)
	}
}

func TestGetTrackedIgnoredFiles_Nested(t *testing.T) {
	_, cleanup := setupTestRepo(t)
	defer cleanup()

	// 1. Create a nested file and track it
	os.MkdirAll("subdir", 0755)
	filePath := filepath.Join("subdir", "temp.log")
	if err := os.WriteFile(filePath, []byte("log"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	exec.Command("git", "add", filePath).Run()
	exec.Command("git", "commit", "-m", "add log").Run()

	// 2. Add a nested .gitignore
	ignorePath := filepath.Join("subdir", ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("*.log\n"), 0644); err != nil {
		t.Fatalf("failed to write nested .gitignore: %v", err)
	}

	// 3. Verify detection
	// The path returned by ls-files should be relative to root, i.e., subdir/temp.log
	ignored, err := GetTrackedIgnoredFiles()
	if err != nil {
		t.Fatalf("GetTrackedIgnoredFiles failed: %v", err)
	}

	// git ls-files returns paths with forward slashes even on windows usually,
	// but let's check what we get.
	expected := "subdir/temp.log"
	
	found := false
	for _, f := range ignored {
		// Normalize separators just in case
		if strings.ReplaceAll(f, "\\", "/") == expected {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected %s to be in ignored list, got %v", expected, ignored)
	}
}
