package git

import "os/exec"

// IsInstalled checks if the git command is available in the system PATH.
// It returns true if git is found, and false otherwise.
func IsInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}
