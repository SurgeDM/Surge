package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDBLifecycle(t *testing.T) {
	// Setup isolated environment
	tempDir := setupTestDB(t)
	defer func() { _ = os.RemoveAll(tempDir) }()
	defer CloseDB()

	// Test GetDB (should be initialized by setupTestDB)
	d, err := GetDB(context.Background())
	if err != nil {
		t.Fatalf("GetDB failed: %v", err)
	}
	if d == nil {
		t.Fatal("GetDB returned nil")
	}

	// Test Singleton
	d2, err := GetDB(context.Background())
	if err != nil {
		t.Fatalf("GetDB 2 failed: %v", err)
	}
	if d != d2 {
		t.Error("GetDB should return the same instance")
	}

	// Test CloseDB
	CloseDB()
	if db != nil {
		t.Error("db variable should be nil after CloseDB")
	}

	// Verify we can re-open after re-configuring path
	Configure(filepath.Join(tempDir, "surge.db"))
	d3, err := GetDB(context.Background())
	if err != nil {
		t.Fatalf("Re-opening GetDB failed: %v", err)
	}
	if d3 == nil {
		t.Fatal("Re-opened DB is nil")
	}
	if d3 == d {
		t.Log("Re-opened DB instance address is same as old closed one (unlikely but possible if pointer reused)")
	}

	// Test tables exist
	tx, err := d3.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("Failed to begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(context.Background(), "SELECT * FROM downloads LIMIT 1")
	if err != nil {
		t.Errorf("Table 'downloads' check failed: %v", err)
	}
	_, err = tx.ExecContext(context.Background(), "SELECT * FROM tasks LIMIT 1")
	if err != nil {
		t.Errorf("Table 'tasks' check failed: %v", err)
	}
}

func TestWithTx_Commit(t *testing.T) {
	tmpDir := setupTestDB(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer CloseDB()

	err := withTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), "INSERT INTO downloads (id, url, dest_path) VALUES (?, ?, ?)", "tx-test-1", "http://tx.com/1", "/tmp/1")
		return err
	})
	if err != nil {
		t.Fatalf("withTx failed: %v", err)
	}

	// Verify data persisted
	d, _ := GetDB(context.Background())
	var url string
	err = d.QueryRowContext(context.Background(), "SELECT url FROM downloads WHERE id = ?", "tx-test-1").Scan(&url)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if url != "http://tx.com/1" {
		t.Errorf("Expected 'http://tx.com/1', got '%s'", url)
	}
}

func TestWithTx_Rollback(t *testing.T) {
	tmpDir := setupTestDB(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer CloseDB()

	// Ensure DB is clean
	d, _ := GetDB(context.Background())
	if _, err := d.ExecContext(context.Background(), "DELETE FROM downloads"); err != nil {
		t.Fatal(err)
	}

	expectedErr := errors.New("intentional error")
	err := withTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), "INSERT INTO downloads (id, url, dest_path) VALUES (?, ?, ?)", "tx-test-2", "http://tx.com/2", "/tmp/2")
		if err != nil {
			return err
		}
		// trigger rollback
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("Expected error %v, got %v", expectedErr, err)
	}

	// Verify data NOT persisted
	var count int
	err = d.QueryRowContext(context.Background(), "SELECT count(*) FROM downloads WHERE id = ?", "tx-test-2").Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 0 {
		t.Error("Transaction should have rolled back, but record found")
	}
}

func TestInitDB_createsDir(t *testing.T) {
	// Setup isolated environment but manually to check dir creation
	tempDir, err := os.MkdirTemp("", "surge-db-init-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Ensure pure state
	dbMu.Lock()
	if db != nil {
		_ = db.Close()
		db = nil
	}
	configured = false
	dbMu.Unlock()

	// Configure
	dbPath := filepath.Join(tempDir, "surge.db")
	Configure(dbPath)

	// GetDB calls initDB
	d, err := GetDB(context.Background())
	if err != nil {
		t.Fatalf("GetDB failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	// Check if database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file not created at %s", dbPath)
	}
}

func TestInitDB_CreatesTasksDownloadIDIndex(t *testing.T) {
	tmpDir := setupTestDB(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer CloseDB()

	d, err := GetDB(context.Background())
	if err != nil {
		t.Fatalf("GetDB failed: %v", err)
	}

	var indexName string
	err = d.QueryRowContext(context.Background(), `
		SELECT name
		FROM sqlite_master
		WHERE type = 'index'
		  AND name = 'idx_tasks_download_id'
	`).Scan(&indexName)
	if err != nil {
		t.Fatalf("failed to query tasks index: %v", err)
	}

	if indexName != "idx_tasks_download_id" {
		t.Fatalf("unexpected index name: %q", indexName)
	}
}
