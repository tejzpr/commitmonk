package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/tejzpr/commitmonk/cmd"
	"github.com/tejzpr/commitmonk/config"
	"github.com/tejzpr/commitmonk/db"
	"github.com/tejzpr/commitmonk/logger"
	"github.com/urfave/cli/v2"
)

func main() {
	// Create CLI app with global verbose flag
	app := &cli.App{
		Name:  "commitmonk",
		Usage: "Automated Git commit tool",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Enable verbose logging",
			},
		},
		Before: func(c *cli.Context) error {
			// Initialize logger with verbose flag
			logger.Init(c.Bool("verbose"))
			return nil
		},
	}

	// Ensure config directory exists
	configDir, err := config.GetConfigDir()
	if err != nil {
		log.Fatalf("Failed to get config directory: %v", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("Failed to create config directory: %v", err)
	}

	// Initialize DB
	dbPath := filepath.Join(configDir, "commitmonk.db")
	database, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Errorf("Warning: Failed to load config: %v", err)
		// Continue with default config
		cfg = config.DefaultConfig()
	}

	// Add commands to app
	app.Commands = []*cli.Command{
		cmd.AddCommand(database, cfg),
		cmd.RemoveCommand(database),
		cmd.ListCommand(database),
		cmd.ConfigCommand(cfg),
		cmd.RunCommand(database, cfg),
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
