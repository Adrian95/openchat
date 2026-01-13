package exporter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitAddAndCommit adds a file to git and commits it
// This is designed to fail gracefully if git is not available or not initialized
func GitAddAndCommit(repoPath, filename, sessionName string) error {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH")
	}

	// Check if directory is a git repository
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository: %s", repoPath)
	}

	// Run git add
	addCmd := exec.Command("git", "add", filename)
	addCmd.Dir = repoPath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s - %w", strings.TrimSpace(string(output)), err)
	}

	// Run git commit
	commitMessage := fmt.Sprintf("Export session: %s", sessionName)
	commitCmd := exec.Command("git", "commit", "-m", commitMessage)
	commitCmd.Dir = repoPath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		outputStr := strings.TrimSpace(string(output))
		// Ignore "nothing to commit" which is not really an error
		if strings.Contains(outputStr, "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %s - %w", outputStr, err)
	}

	return nil
}

// InitGitRepo initializes a git repository in the given path
func InitGitRepo(repoPath string) error {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if already a git repository
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil // Already initialized
	}

	// Run git init
	initCmd := exec.Command("git", "init")
	initCmd.Dir = repoPath
	if output, err := initCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %s - %w", strings.TrimSpace(string(output)), err)
	}

	// Create .gitignore
	gitignore := filepath.Join(repoPath, ".gitignore")
	gitignoreContent := "# Temporary files\n*.tmp\n*.bak\n.DS_Store\n"
	if err := os.WriteFile(gitignore, []byte(gitignoreContent), 0644); err != nil {
		// Non-fatal error
		return nil
	}

	return nil
}

// IsGitRepo checks if the given path is a git repository
func IsGitRepo(repoPath string) bool {
	gitDir := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitDir)
	return err == nil && info.IsDir()
}

// GetGitStatus returns the git status of the repository
func GetGitStatus(repoPath string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git not found in PATH")
	}

	statusCmd := exec.Command("git", "status", "--short")
	statusCmd.Dir = repoPath
	output, err := statusCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GitPush pushes commits to the remote repository
func GitPush(repoPath string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH")
	}

	pushCmd := exec.Command("git", "push")
	pushCmd.Dir = repoPath
	if output, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %s - %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}
