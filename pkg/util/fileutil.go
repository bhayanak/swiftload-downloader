package util

import (
	"fmt"
	"os"
)

// CreateOrOpenFile creates a new file or opens an existing one for writing.
// If the file already exists, it is opened with read/write access.
func CreateOrOpenFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot open output file: %w", err)
	}
	return f, nil
}

// PreallocateFile truncates the file to the given size for pre-allocation.
func PreallocateFile(f *os.File, size int64) error {
	if err := f.Truncate(size); err != nil {
		return fmt.Errorf("file pre-allocation failed: %w", err)
	}
	return nil
}

// SyncAndClose fsyncs and closes the file.
func SyncAndClose(f *os.File) error {
	if err := f.Sync(); err != nil {
		return fmt.Errorf("file sync failed: %w", err)
	}
	return f.Close()
}
