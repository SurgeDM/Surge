package store

import (
	"encoding/gob"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/SurgeDM/Surge/internal/types"
)

// InspectDirectory reads one index without configuring the global store,
// sanitizing files, deleting unsupported versions, or creating directories.
func InspectDirectory(dir string) ([]types.DownloadRecord, error) {
	file, err := os.Open(filepath.Join(dir, "master.gob"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 32<<20 {
		return nil, errors.New("index exceeds diagnostic size limit")
	}
	var state MasterState
	if err := gob.NewDecoder(io.LimitReader(file, 32<<20)).Decode(&state); err != nil {
		return nil, err
	}
	if state.Version != 2 {
		return nil, errors.New("unsupported state version")
	}
	return state.Downloads, nil
}
