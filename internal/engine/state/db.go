package state

import (
	"database/sql"
	"fmt"
	"log"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db         *sql.DB
	dbMu       sync.Mutex
	dbPath     string
	configured bool
)

// Configure sets the path for the SQLite database
func Configure(path string) {
	dbMu.Lock()
	defer dbMu.Unlock()
	dbPath = path
	configured = true
}

// initDBLocked initialises the database connection.
// Caller must hold dbMu.
func initDBLocked() error {
	if db != nil {
		return nil
	}

	if !configured || dbPath == "" {
		return fmt.Errorf("state database not configured: call state.Configure() first")
	}

	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode and busy_timeout for concurrent reader-writer access
	// (required now that the processing layer's event worker writes from a goroutine)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("failed to set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("failed to set busy_timeout: %w", err)
	}

	// Create tables
	query := `
	CREATE TABLE IF NOT EXISTS downloads (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		dest_path TEXT NOT NULL,
		filename TEXT,
		status TEXT,
		total_size INTEGER,
		downloaded INTEGER,
		url_hash TEXT,
		created_at INTEGER,
		paused_at INTEGER,
		completed_at INTEGER,
		time_taken INTEGER,
		mirrors TEXT,
		chunk_bitmap BLOB,
		actual_chunk_size INTEGER,
		avg_speed REAL,
		file_hash TEXT
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		download_id TEXT,
		offset INTEGER,
		length INTEGER,
		FOREIGN KEY(download_id) REFERENCES downloads(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_download_id ON tasks(download_id);
	`

	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	if err := ensureDownloadsSchema(); err != nil {
		return fmt.Errorf("failed to ensure schema: %w", err)
	}

	return nil
}

func initDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	return initDBLocked()
}

// ensureDownloadsSchema checks if required columns exist in the downloads table and adds them if missing.
func ensureDownloadsSchema() error {
	rows, err := db.Query("PRAGMA table_info(downloads)")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	existingColumns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		existingColumns[name] = true
	}

	columnsToAdd := []struct {
		name string
		def  string
	}{
		{"mirrors", "TEXT"},
		{"chunk_bitmap", "BLOB"},
		{"actual_chunk_size", "INTEGER"},
		{"avg_speed", "REAL"},
		{"file_hash", "TEXT"},
	}

	for _, col := range columnsToAdd {
		if !existingColumns[col.name] {
			alterQuery := fmt.Sprintf("ALTER TABLE downloads ADD COLUMN %s %s", col.name, col.def)
			if _, err := db.Exec(alterQuery); err != nil {
				log.Printf("Failed to add column %s: %v", col.name, err)
			}
		}
	}

	return nil
}

func CloseDB() {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil {
		_ = db.Close()
		db = nil
	}
	dbPath = ""
	configured = false
}

// GetDB is safe for concurrent use; it lazily opens the database so callers
// need not coordinate initialisation order with Configure().
func GetDB() (*sql.DB, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		if err := initDBLocked(); err != nil {
			return nil, err
		}
	}
	return db, nil
}

// Helper to ensure DB is initialized and return it
func getDBHelper() *sql.DB {
	d, err := GetDB()
	if err != nil {
		log.Printf("State DB Error: %v", err)
		return nil
	}
	return d
}

// Transaction helper
func withTx(fn func(*sql.Tx) error) error {
	d := getDBHelper()
	if d == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := d.Begin()
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
