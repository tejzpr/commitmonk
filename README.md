# Commitmonk 🧘‍♂️

Commitmonk is an automated Git commit tool that helps you maintain a clean commit history by automatically committing changes at specified intervals. It can use OpenAI or compatible LLM APIs to generate meaningful commit messages based on the changes.

## Features

- Automatically monitor Git repositories for changes
- Schedule commits at customizable intervals
- Optionally auto-stage and auto-push changes
- Generate commit messages using AI (OpenAI or compatible APIs)
- Support for static commit messages when AI is not available
- Exclude files from being committed using glob patterns

## Installation

### From Source

Requires Go 1.17 or later.

```bash
# Clone the repository
git clone https://github.com/tejzpr/commitmonk.git
cd commitmonk

# Build the binary
go build -o commitmonk .

# Install to a directory in your PATH (optional)
sudo mv commitmonk /usr/local/bin/
```

## Usage

### Global Options

- `--verbose`, `-v`: Enable verbose logging (default: silent operation)

### Configuration

Set up the configuration and LLM API credentials:

```bash
commitmonk config
```

This will prompt you for:
- Default commit interval
- OpenAI API base URL
- API key
- Model name

### Adding a Repository

Register a repository for automated commits:

```bash
commitmonk add /path/to/repo --every 5m --autopush
```

Options:
- `--every`, `-e`: Commit interval (e.g., 5m, 1h, 30m)
- `--no-autoadd`: Disable automatic staging of changes (auto-add is enabled by default)
- `--autopush`: Automatically push commits to remote
- `--message`, `-m`: Static commit message (used when LLM is not configured)
- `--exclude`: Comma-separated glob patterns to exclude from commits (e.g., "*.log,tmp/*")

### Listing Registered Repositories

```bash
commitmonk list
```

This will show all registered repositories with their IDs, paths, and settings.

### Removing a Repository

Remove by path:
```bash
commitmonk remove /path/to/repo
```

Or remove by ID (as shown in the list command):
```bash
commitmonk remove 3
```

### Running the Scheduler

Start the commit scheduler:

```bash
commitmonk run
```

For detailed logs:
```bash
commitmonk run -v
```

Press `Ctrl+C` to stop the scheduler.

## Examples

```bash
# Register a repository with 10-minute interval and auto-pushing (auto-staging enabled by default)
commitmonk add ~/projects/my-project --every 10m --autopush

# Register a repository with auto-staging disabled
commitmonk add ~/projects/another-project --every 30m --no-autoadd --message "Auto-commit" --exclude "*.log,tmp/*"

# Start the scheduler with verbose logging
commitmonk run -v

# List all registered repositories
commitmonk list
```

## License

MIT