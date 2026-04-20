package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SurgeDM/Surge/internal/backup"
	"github.com/SurgeDM/Surge/internal/core"
)

func resolveTransferService() (core.TransferService, bool, error) {
	baseURL, token, err := resolveAPIConnection(false)
	if err != nil {
		return nil, false, err
	}
	if baseURL != "" {
		return core.NewRemoteTransferService(baseURL, token), true, nil
	}
	return core.NewLocalTransferService(GlobalService, Version), false, nil
}

func ensureExportPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "surge-export" + backup.BundleExtension
	}
	if strings.HasSuffix(path, backup.BundleExtension) {
		return path
	}
	return path + backup.BundleExtension
}

func printJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printImportPreview(preview *backup.ImportPreview) {
	fmt.Printf("Root: %s\n", preview.RootDir)
	fmt.Printf("Imports: %v\n", preview.ImportsByStatus)
	fmt.Printf("Duplicates skipped: %d\n", preview.DuplicatesSkipped)
	fmt.Printf("Renamed items: %d\n", preview.RenamedItems)
	fmt.Printf("Downgraded to queue: %d\n", preview.ResumableJobsDowngradedToQueue)
}

func printImportResult(result *backup.ImportResult) {
	if result == nil {
		return
	}
	if result.Preview != nil {
		printImportPreview(result.Preview)
	}
	fmt.Printf("Imported: %d\n", result.Imported)
	if result.LogsRestored > 0 {
		fmt.Printf("Logs restored: %d\n", result.LogsRestored)
	}
}

func exportBundle(ctx context.Context, transfer core.TransferService, path string, opts backup.ExportOptions) (*backup.Manifest, error) {
	target := ensureExportPath(path)
	if err := os.MkdirAll(filepath.Dir(filepath.Clean(target)), 0o755); err != nil && filepath.Dir(filepath.Clean(target)) != "." {
		return nil, err
	}
	file, err := os.Create(target)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	manifest, err := transfer.Export(ctx, opts, file)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func previewBundle(ctx context.Context, transfer core.TransferService, path string, opts backup.ImportOptions) (*backup.ImportPreview, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return transfer.PreviewImport(ctx, file, opts)
}

func applyBundle(ctx context.Context, transfer core.TransferService, path string, opts backup.ImportOptions) (*backup.ImportResult, error) {
	var src io.Reader
	if strings.TrimSpace(opts.SessionID) == "" {
		file, err := os.Open(filepath.Clean(path))
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		src = file
	}
	return transfer.ApplyImport(ctx, src, opts)
}
