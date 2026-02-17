package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a temporary directory, initializes a git repo,
// and returns the path and a cleanup function.
// It also changes the current working directory to the temp repo.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current wd: %v", err)
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}

	cmd := exec.Command("git", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %s", out)
	}

	// Configure git user for commits
	exec.Command("git", "config", "user.email", "you@example.com").Run()
	exec.Command("git", "config", "user.name", "Your Name").Run()

	// Initial commit to allow push behavior (even though we'll mock/ignore remote for now,
	// checking IsTracked requires valid repo state)
	if err := os.WriteFile("README.md", []byte("init"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	exec.Command("git", "add", "README.md").Run()
	exec.Command("git", "commit", "-m", "init").Run()
	
	// We need a remote for Push to work, or we need to stub Push.
	// Since we are testing internal logic, we can fake a remote by creating a bare repo next door.
	remoteDir := t.TempDir()
	remotePath := filepath.Join(remoteDir, "remote.git")
	exec.Command("git", "init", "--bare", remotePath).Run()
	exec.Command("git", "remote", "add", "origin", remotePath).Run()
	exec.Command("git", "push", "-u", "origin", "master").Run()

	cleanup := func() {
		os.Chdir(originalWd)
	}

	return tmpDir, cleanup
}

func TestSave_Normal(t *testing.T) {
	_, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create a new file
	if err := os.WriteFile("new.txt", []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Call Save
	err := Save([]string{"new.txt"}, "feat: add new file")
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify commit
	out, _ := exec.Command("git", "log", "-1", "--pretty=%s").Output()
	if strings.TrimSpace(string(out)) != "feat: add new file" {
		t.Errorf("expected commit message 'feat: add new file', got '%s'", strings.TrimSpace(string(out)))
	}
}

func TestSave_GitignorePrecedence(t *testing.T) {
	_, cleanup := setupTestRepo(t)
	defer cleanup()

	// 1. Create a file and track it
	secretFile := "secret.key"
	if err := os.WriteFile(secretFile, []byte("12345"), 0644); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}
	exec.Command("git", "add", secretFile).Run()
	exec.Command("git", "commit", "-m", "add secret").Run()
	exec.Command("git", "push").Run()

	// 2. Now modify .gitignore to exclude it
	if err := os.WriteFile(".gitignore", []byte("*.key\n"), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	// 3. Create another normal file
	normalFile := "normal.txt"
	if err := os.WriteFile(normalFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write normal file: %v", err)
	}

	// 4. Call Save with both .gitignore and normal.txt
	// This should trigger the smart workflow:
	// - Commit 1: .gitignore
	// - Commit 2: untrack secret.key
	// - Commit 3: normal.txt
	err := Save([]string{".gitignore", normalFile}, "feat: add normal file")
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 5. Verify commit history (reverse order)
	// head: feat: add normal file
	// head-1: chore(gitignore): untrack ignored files
	// head-2: chore(gitignore): update ignore rules
	
	commits, _ := exec.Command("git", "log", "-3", "--pretty=%s").Output()
	lines := strings.Split(strings.TrimSpace(string(commits)), "\n")
	
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 commits, got %d", len(lines))
	}

	if lines[0] != "feat: add normal file" {
		t.Errorf("expected last commit to be 'feat: add normal file', got '%s'", lines[0])
	}
	if lines[1] != "chore(gitignore): untrack ignored files" {
		t.Errorf("expected cleanup commit, got '%s'", lines[1])
	}
	if lines[2] != "chore(gitignore): update ignore rules" {
		t.Errorf("expected gitignore commit, got '%s'", lines[2])
	}

	// 6. Verify file status
	// secret.key should be gone from index (untracked) but present on disk?
	// actually git rm --cached keeps it on disk.
	if _, err := os.Stat(secretFile); os.IsNotExist(err) {
		t.Errorf("secret.key should still exist on disk")
	}
	
	// Check if it's tracked
	// It should NOT be tracked now
	cmd := exec.Command("git", "ls-files", "--error-unmatch", secretFile)
	if err := cmd.Run(); err == nil {
		t.Errorf("secret.key should be untracked now")
	}
}
