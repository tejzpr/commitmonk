package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// Task represents a repository task in the database
type Task struct {
	ID              int64
	Path            string
	Every           string
	AutoAdd         bool
	AutoPush        bool
	StaticMsg       string
	ExcludePatterns string
}

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
}

// InitDB initializes the SQLite database
func InitDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create tasks table if it doesn't exist
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY,
		path TEXT UNIQUE NOT NULL,
		every TEXT NOT NULL,
		auto_add BOOLEAN NOT NULL,
		auto_push BOOLEAN NOT NULL,
		static_msg TEXT,
		exclude_patterns TEXT
	);`

	_, err = conn.Exec(createTableSQL)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// AddTask adds a new repository task to the database
func (db *DB) AddTask(task Task) error {
	// Replace existing task if path already exists
	stmt, err := db.conn.Prepare(`
		INSERT OR REPLACE INTO tasks 
		(path, every, auto_add, auto_push, static_msg, exclude_patterns)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		task.Path,
		task.Every,
		task.AutoAdd,
		task.AutoPush,
		task.StaticMsg,
		task.ExcludePatterns,
	)
	if err != nil {
		return fmt.Errorf("failed to add task: %w", err)
	}

	return nil
}

// RemoveTask removes a repository task from the database
func (db *DB) RemoveTask(path string) error {
	stmt, err := db.conn.Prepare("DELETE FROM tasks WHERE path = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	result, err := stmt.Exec(path)
	if err != nil {
		return fmt.Errorf("failed to remove task: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no task found for path: %s", path)
	}

	return nil
}

// RemoveTaskByID removes a repository task from the database by ID
func (db *DB) RemoveTaskByID(id int64) error {
	stmt, err := db.conn.Prepare("DELETE FROM tasks WHERE id = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	result, err := stmt.Exec(id)
	if err != nil {
		return fmt.Errorf("failed to remove task: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no task found with ID: %d", id)
	}

	return nil
}

// GetAllTasks retrieves all tasks from the database
func (db *DB) GetAllTasks() ([]Task, error) {
	rows, err := db.conn.Query(`
		SELECT id, path, every, auto_add, auto_push, static_msg, exclude_patterns
		FROM tasks
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var task Task
		err := rows.Scan(
			&task.ID,
			&task.Path,
			&task.Every,
			&task.AutoAdd,
			&task.AutoPush,
			&task.StaticMsg,
			&task.ExcludePatterns,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		tasks = append(tasks, task)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating through rows: %w", err)
	}

	return tasks, nil
}

// GetTask retrieves a specific task by path
func (db *DB) GetTask(path string) (*Task, error) {
	stmt, err := db.conn.Prepare(`
		SELECT id, path, every, auto_add, auto_push, static_msg, exclude_patterns
		FROM tasks
		WHERE path = ?
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	var task Task
	err = stmt.QueryRow(path).Scan(
		&task.ID,
		&task.Path,
		&task.Every,
		&task.AutoAdd,
		&task.AutoPush,
		&task.StaticMsg,
		&task.ExcludePatterns,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no task found for path: %s", path)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return &task, nil
}
