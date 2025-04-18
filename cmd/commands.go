package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tejzpr/commitmonk/config"
	"github.com/tejzpr/commitmonk/db"
	"github.com/tejzpr/commitmonk/scheduler"
	"github.com/urfave/cli/v2"
)

// AddCommand registers a new repository to monitor
func AddCommand(database *db.DB, cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Register a repository for automated commits",
		ArgsUsage: "<path>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "no-autoadd",
				Usage: "Disable automatic staging of changes (auto-add enabled by default)",
			},
			&cli.BoolFlag{
				Name:  "autopush",
				Usage: "Automatically push after commit",
			},
			&cli.StringFlag{
				Name:    "every",
				Aliases: []string{"e"},
				Usage:   "Commit interval (>=1m, default: 5m)",
				Value:   cfg.DefaultInterval,
			},
			&cli.StringFlag{
				Name:    "message",
				Aliases: []string{"m"},
				Usage:   "Static commit message when LLM is not configured",
			},
			&cli.StringFlag{
				Name:  "exclude",
				Usage: "Comma-separated list of glob patterns to ignore",
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("path argument required")
			}

			// Get absolute path to the repository
			repoPath := c.Args().Get(0)
			absPath, err := filepath.Abs(repoPath)
			if err != nil {
				return fmt.Errorf("failed to get absolute path: %w", err)
			}

			// Verify directory exists and contains a git repository
			if _, err := os.Stat(filepath.Join(absPath, ".git")); os.IsNotExist(err) {
				return fmt.Errorf("%s is not a git repository", absPath)
			}

			// Validate interval
			interval := c.String("every")
			duration, err := time.ParseDuration(interval)
			if err != nil {
				return fmt.Errorf("invalid interval format: %w", err)
			}
			if duration < time.Minute {
				return fmt.Errorf("interval must be at least 1 minute")
			}

			// Check if message is required
			staticMsg := c.String("message")
			if staticMsg == "" && cfg.LLM.APIKey == "" {
				return fmt.Errorf("commit message is required when LLM is not configured. Use --message to provide one")
			}

			// Create task - note the negation of no-autoadd flag
			task := db.Task{
				Path:            absPath,
				Every:           interval,
				AutoAdd:         !c.Bool("no-autoadd"), // Default is true if no-autoadd is not specified
				AutoPush:        c.Bool("autopush"),
				StaticMsg:       staticMsg,
				ExcludePatterns: c.String("exclude"),
			}

			// Add to database
			if err := database.AddTask(task); err != nil {
				return fmt.Errorf("failed to register repository: %w", err)
			}

			fmt.Printf("Registered %s (every %s", absPath, interval)
			// Update display to reflect the changed default behavior
			if !task.AutoAdd {
				fmt.Print(", auto-add disabled")
			}
			if task.AutoPush {
				fmt.Print(", auto-push enabled")
			}
			if task.StaticMsg != "" {
				fmt.Printf(", message=\"%s\"", task.StaticMsg)
			}
			if task.ExcludePatterns != "" {
				fmt.Printf(", exclude=%s", task.ExcludePatterns)
			}
			fmt.Println(")")

			return nil
		},
	}
}

// RemoveCommand unregisters a repository
func RemoveCommand(database *db.DB) *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "Unregister a repository by path or ID",
		ArgsUsage: "<path or id>",
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("path or id argument required")
			}

			arg := c.Args().Get(0)

			// Check if the argument is a numeric ID
			var id int64
			_, err := fmt.Sscanf(arg, "%d", &id)
			if err == nil {
				// Argument is a numeric ID
				err = database.RemoveTaskByID(id)
				if err != nil {
					return fmt.Errorf("failed to unregister repository with ID %d: %w", id, err)
				}
				fmt.Printf("Unregistered repository with ID %d\n", id)
				return nil
			}

			// Argument is a path
			absPath, err := filepath.Abs(arg)
			if err != nil {
				return fmt.Errorf("failed to get absolute path: %w", err)
			}

			if err := database.RemoveTask(absPath); err != nil {
				return fmt.Errorf("failed to unregister repository: %w", err)
			}

			fmt.Printf("Unregistered %s\n", absPath)
			return nil
		},
	}
}

// ListCommand lists all registered repositories
func ListCommand(database *db.DB) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all registered repositories",
		Action: func(c *cli.Context) error {
			tasks, err := database.GetAllTasks()
			if err != nil {
				return fmt.Errorf("failed to list repositories: %w", err)
			}

			if len(tasks) == 0 {
				fmt.Println("No repositories registered")
				return nil
			}

			fmt.Println("Registered repositories:")
			for _, task := range tasks {
				fmt.Printf("[ID: %d] %s (every %s", task.ID, task.Path, task.Every)
				if task.AutoAdd {
					fmt.Print(", auto-add enabled")
				} else {
					fmt.Print(", auto-add disabled")
				}
				if task.AutoPush {
					fmt.Print(", auto-push enabled")
				}
				if task.StaticMsg != "" {
					fmt.Printf(", message=\"%s\"", task.StaticMsg)
				}
				if task.ExcludePatterns != "" {
					fmt.Printf(", exclude=%s", task.ExcludePatterns)
				}
				fmt.Println(")")
			}

			return nil
		},
	}
}

// ConfigCommand sets up the LLM configuration
func ConfigCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Configure default settings and LLM credentials",
		Action: func(c *cli.Context) error {
			scanner := bufio.NewScanner(os.Stdin)

			// Configure default interval
			fmt.Printf("Default commit interval (current: %s): ", cfg.DefaultInterval)
			scanner.Scan()
			input := strings.TrimSpace(scanner.Text())
			if input != "" {
				duration, err := time.ParseDuration(input)
				if err != nil {
					return fmt.Errorf("invalid interval format: %w", err)
				}
				if duration < time.Minute {
					return fmt.Errorf("interval must be at least 1 minute")
				}
				cfg.DefaultInterval = input
			}

			// Configure LLM
			fmt.Printf("OpenAI-compatible API Base URL (current: %s): ", cfg.LLM.BaseURL)
			scanner.Scan()
			input = strings.TrimSpace(scanner.Text())
			if input != "" {
				cfg.LLM.BaseURL = input
			}

			fmt.Printf("API key (current: %s): ", maskAPIKey(cfg.LLM.APIKey))
			scanner.Scan()
			input = strings.TrimSpace(scanner.Text())
			if input != "" {
				cfg.LLM.APIKey = input
			}

			fmt.Printf("Model (current: %s): ", cfg.LLM.Model)
			scanner.Scan()
			input = strings.TrimSpace(scanner.Text())
			if input != "" {
				cfg.LLM.Model = input
			}

			// Save configuration
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			fmt.Println("Configuration saved successfully")
			return nil
		},
	}
}

// maskAPIKey masks most of the API key for display
func maskAPIKey(key string) string {
	if key == "" {
		return "<not set>"
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// RunCommand starts the scheduler
func RunCommand(database *db.DB, cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Start the commit scheduler",
		Action: func(c *cli.Context) error {
			runner := scheduler.NewTaskRunner(database, cfg)
			if err := runner.Start(); err != nil {
				return fmt.Errorf("failed to start scheduler: %w", err)
			}

			fmt.Println("Monitoring changes. Press Ctrl+C to stop.")

			// Set up signal handling for graceful shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			// Wait for interrupt signal
			<-sigCh
			fmt.Println("\nShutting down...")

			runner.Stop()
			return nil
		},
	}
}
