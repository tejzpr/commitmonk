package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/tejzpr/commitmonk/config"
	"github.com/tejzpr/commitmonk/db"
	"github.com/tejzpr/commitmonk/git"
	"github.com/tejzpr/commitmonk/llm"
	"github.com/tejzpr/commitmonk/logger"
)

// TaskRunner handles the execution of repository tasks
type TaskRunner struct {
	database  *db.DB
	llmClient *llm.Client
	stopCh    chan struct{}
	tasks     map[int64]*taskState
	// Add lastCheck timestamp to track when we last checked for DB changes
	lastCheck time.Time
}

// taskState tracks the state of a running task
type taskState struct {
	task    db.Task
	nextRun time.Time
}

// NewTaskRunner creates a new task runner
func NewTaskRunner(database *db.DB, cfg *config.Config) *TaskRunner {
	return &TaskRunner{
		database:  database,
		llmClient: llm.NewClient(cfg.LLM),
		stopCh:    make(chan struct{}),
		tasks:     make(map[int64]*taskState),
		lastCheck: time.Now(),
	}
}

// Start begins the task scheduler
func (r *TaskRunner) Start() error {
	logger.Println("Starting task scheduler...")

	// Initial load of tasks
	if err := r.loadTasks(); err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Start the main scheduling loop
	go r.run()

	return nil
}

// Stop stops the task scheduler
func (r *TaskRunner) Stop() {
	close(r.stopCh)
	logger.Println("Task scheduler stopped")
}

// loadTasks loads all tasks from the database
func (r *TaskRunner) loadTasks() error {
	tasks, err := r.database.GetAllTasks()
	if err != nil {
		return err
	}

	// Create map of current task IDs for change detection
	currentTaskIDs := make(map[int64]bool)

	// Process each task from the database
	for _, task := range tasks {
		currentTaskIDs[task.ID] = true

		// Check if we already have this task
		if existingState, exists := r.tasks[task.ID]; exists {
			// Update the task data but keep the next run time if it's still in the future
			existingState.task = task
			logger.Printf("Updated task: %s (ID: %d, every %s)", task.Path, task.ID, task.Every)
		} else {
			// This is a new task, schedule its first run
			duration, err := time.ParseDuration(task.Every)
			if err != nil {
				logger.Printf("Warning: Invalid duration for task %d (%s): %v", task.ID, task.Path, err)
				continue
			}

			r.tasks[task.ID] = &taskState{
				task:    task,
				nextRun: time.Now().Add(duration), // Schedule next run
			}
			logger.Printf("Loaded new task: %s (ID: %d, every %s)", task.Path, task.ID, task.Every)
		}
	}

	// Identify and remove tasks no longer in database
	for id := range r.tasks {
		if !currentTaskIDs[id] {
			logger.Printf("Removing task with ID %d as it's no longer in the database", id)
			delete(r.tasks, id)
		}
	}

	// Update lastCheck timestamp
	r.lastCheck = time.Now()

	return nil
}

// checkForChanges checks if there have been changes in the task list
func (r *TaskRunner) checkForChanges() error {
	// Check for updates every 10 seconds
	if time.Since(r.lastCheck) < 10*time.Second {
		return nil
	}

	logger.Println("Checking for task updates...")
	return r.loadTasks()
}

// run is the main scheduler loop
func (r *TaskRunner) run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			// Check for task list changes
			if err := r.checkForChanges(); err != nil {
				logger.Printf("Error checking for task updates: %v", err)
			}

			r.processTasks()
		}
	}
}

// processTasks checks for and executes due tasks
func (r *TaskRunner) processTasks() {
	now := time.Now()

	for id, state := range r.tasks {
		if now.After(state.nextRun) {
			// Execute task
			go r.executeTask(state.task)

			// Update next run time
			duration, err := time.ParseDuration(state.task.Every)
			if err != nil {
				logger.Printf("Error parsing duration for task %d: %v", id, err)
				delete(r.tasks, id) // Remove invalid task
				continue
			}
			r.tasks[id].nextRun = now.Add(duration)
		}
	}
}

// executeTask processes a single repository task
func (r *TaskRunner) executeTask(task db.Task) {
	logger.Printf("Executing task for repository: %s", task.Path)

	// Create repository manager
	repoManager, err := git.NewRepoManager(task.Path)
	if err != nil {
		logger.Errorf("Error opening repository %s: %v", task.Path, err)
		return
	}

	// Check for changes
	hasChanges, err := repoManager.HasChanges()
	if err != nil {
		logger.Errorf("Error checking for changes in %s: %v", task.Path, err)
		return
	}

	if !hasChanges {
		logger.Printf("No changes detected in %s, skipping", task.Path)
		return
	}

	// Stage changes if configured
	if task.AutoAdd {
		logger.Printf("Auto-staging changes in %s", task.Path)
		if err := repoManager.StageChanges(task.ExcludePatterns); err != nil {
			logger.Errorf("Error staging changes in %s: %v", task.Path, err)
			return
		}
	} else {
		// If auto-add is not enabled, check if there are already staged changes
		hasStagedChanges, err := repoManager.HasStagedChanges()
		if err != nil {
			logger.Errorf("Error checking for staged changes in %s: %v", task.Path, err)
			return
		}

		// If no staged changes and auto-add is disabled, skip this task
		if !hasStagedChanges {
			logger.Printf("No staged changes in %s and auto-add is disabled, skipping", task.Path)
			return
		}
	}

	// Get diff for LLM
	diff, err := repoManager.GetDiff()
	if err != nil {
		logger.Errorf("Error getting diff for %s: %v", task.Path, err)
		return
	}

	// Determine commit message
	var commitMsg string

	// If LLM is configured, always try to use it first regardless of static message
	if r.llmClient.HasCredentials() {
		logger.Printf("Generating commit message using LLM for %s", task.Path)
		commitMsg, err = r.llmClient.GenerateCommitMessage(diff)
		if err != nil {
			logger.Errorf("Error generating commit message for %s: %v", task.Path, err)
			// Fall back to static message if provided
			if task.StaticMsg != "" {
				logger.Printf("Falling back to static message for %s", task.Path)
				commitMsg = task.StaticMsg
			} else {
				logger.Errorf("LLM failed and no static message configured for %s, cannot commit", task.Path)
				return // Don't commit if no message is available
			}
		}
	} else if task.StaticMsg != "" {
		// Use static message if LLM is not configured
		logger.Printf("Using configured static message for %s", task.Path)
		commitMsg = task.StaticMsg
	} else {
		logger.Errorf("No LLM credentials and no static message configured for %s, cannot commit", task.Path)
		return // Don't commit if no message is available
	}

	// Commit changes
	err = repoManager.Commit(commitMsg)
	if err != nil {
		if strings.Contains(err.Error(), "no staged changes") {
			logger.Printf("No staged changes to commit in %s", task.Path)
		} else {
			logger.Errorf("Error committing changes in %s: %v", task.Path, err)
		}
		return
	}
	logger.Printf("Created commit in %s: %s", task.Path, commitMsg)

	// Push if configured
	if task.AutoPush {
		logger.Printf("Auto-pushing commits in %s", task.Path)
		if err := repoManager.Push(); err != nil {
			logger.Errorf("Error pushing changes in %s: %v", task.Path, err)
			return
		}
		logger.Printf("Successfully pushed commits in %s", task.Path)
	}
}
