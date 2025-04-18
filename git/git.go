package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gobwas/glob"
)

// RepoManager handles git operations for a repository
type RepoManager struct {
	path string
	repo *git.Repository
}

// NewRepoManager creates a new repository manager
func NewRepoManager(repoPath string) (*RepoManager, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	repo, err := git.PlainOpen(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	return &RepoManager{
		path: absPath,
		repo: repo,
	}, nil
}

// HasChanges checks if the repository has any changes (staged or unstaged)
func (r *RepoManager) HasChanges() (bool, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	for _, fileStatus := range status {
		if fileStatus.Worktree != git.Unmodified || fileStatus.Staging != git.Unmodified {
			return true, nil
		}
	}
	return false, nil
}

// HasStagedChanges checks if there are any staged changes
func (r *RepoManager) HasStagedChanges() (bool, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	for _, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified {
			return true, nil
		}
	}
	return false, nil
}

// StageChanges stages all changes except those matching exclude patterns
func (r *RepoManager) StageChanges(excludePatterns string) error {
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// First stage all changes
	err = wt.AddGlob(".")
	if err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	// If exclusion patterns are provided, unstage matching files
	if excludePatterns != "" {
		patterns := strings.Split(excludePatterns, ",")
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}

			g, err := glob.Compile(pattern)
			if err != nil {
				return fmt.Errorf("invalid exclude pattern '%s': %w", pattern, err)
			}

			status, err := wt.Status()
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}

			for filePath := range status {
				if g.Match(filePath) {
					// Create a reset option with file path in the Paths slice
					// Note: We can't directly pass the file path to Reset,
					// but need to use git.ResetOptions as documented
					err = wt.RemoveGlob(filePath)
					if err != nil {
						return fmt.Errorf("failed to unstage file %s: %w", filePath, err)
					}
				}
			}
		}
	}

	return nil
}

// GetDiff returns the diff of staged changes
func (r *RepoManager) GetDiff() (string, error) {
	// First try using git executable if available
	if diffStr, err := r.getSystemGitDiff(); err == nil {
		return diffStr, nil
	}

	// Fall back to go-git implementation if git executable is not available
	wt, err := r.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	var buf bytes.Buffer
	for filePath, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified {
			// Use string representation for status code
			stagingStatus := "Modified"
			switch fileStatus.Staging {
			case git.Untracked:
				stagingStatus = "Untracked"
			case git.Modified:
				stagingStatus = "Modified"
			case git.Added:
				stagingStatus = "Added"
			case git.Deleted:
				stagingStatus = "Deleted"
			case git.Renamed:
				stagingStatus = "Renamed"
			case git.Copied:
				stagingStatus = "Copied"
			case git.UpdatedButUnmerged:
				stagingStatus = "Conflicted"
			}
			fmt.Fprintf(&buf, "File: %s (Status: %s)\n", filePath, stagingStatus)
		}
	}

	// We're only getting basic status info since go-git doesn't provide a straightforward
	// way to get the full diff content similar to 'git diff --staged'
	return buf.String(), nil
}

// getSystemGitDiff attempts to get diff using the git executable
func (r *RepoManager) getSystemGitDiff() (string, error) {
	// Check if git is installed
	_, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git executable not found: %w", err)
	}

	// Execute git diff --staged to get the diff of staged changes
	cmd := exec.Command("git", "diff", "--staged")
	cmd.Dir = r.path // Set working directory to repository path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run git diff: %w", err)
	}

	// If there's no diff, we might need to check if there are unstaged changes
	if len(output) == 0 {
		// Try getting unstaged changes
		cmd = exec.Command("git", "diff")
		cmd.Dir = r.path
		output, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to run git diff for unstaged changes: %w", err)
		}
	}

	return string(output), nil
}

// Commit creates a new commit with the given message
func (r *RepoManager) Commit(message string) error {
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Check if there are staged changes
	hasStagedChanges, err := r.HasStagedChanges()
	if err != nil {
		return fmt.Errorf("failed to check for staged changes: %w", err)
	}

	if !hasStagedChanges {
		// Auto-stage all changes if no staged changes exist
		if err := wt.AddGlob("."); err != nil {
			return fmt.Errorf("failed to stage changes: %w", err)
		}

		// Check again if we have staged changes after auto-staging
		hasStagedChanges, err = r.HasStagedChanges()
		if err != nil {
			return fmt.Errorf("failed to check for staged changes: %w", err)
		}

		if !hasStagedChanges {
			return fmt.Errorf("no staged changes to commit")
		}
	}

	_, err = wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Commitmonk",
			Email: "commitmonk@automated.tool",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	return nil
}

// Push pushes commits to the remote repository
func (r *RepoManager) Push() error {
	// Get the current branch
	head, err := r.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Create proper RefSpec
	refSpec := config.RefSpec(head.Name().String() + ":" + head.Name().String())

	// Push to remote
	err = r.repo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{refSpec},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}
