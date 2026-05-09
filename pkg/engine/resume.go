package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ResumeState stores download metadata for crash recovery and resume.
type ResumeState struct {
	Version      int           `json:"version"`
	URL          string        `json:"url"`
	OutputFile   string        `json:"output_file"`
	TotalSize    int64         `json:"total_size"`
	ETag         string        `json:"etag,omitempty"`
	LastModified string        `json:"last_modified,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	Chunks       []ChunkState  `json:"chunks"`
	Checksum     *ChecksumInfo `json:"checksum,omitempty"`
}

// ChunkState tracks progress of a single download chunk.
type ChunkState struct {
	Start      int64 `json:"start"`
	End        int64 `json:"end"`
	Downloaded int64 `json:"downloaded"`
	Done       bool  `json:"done"`
}

// ChecksumInfo stores expected checksum for verification.
type ChecksumInfo struct {
	Algorithm string `json:"algorithm"`
	Expected  string `json:"expected"`
}

// resumeFilePath returns the sidecar metadata path for a given output file.
func resumeFilePath(outputPath string) string {
	return outputPath + ".gdown.json"
}

// saveResumeState writes the resume state to the sidecar JSON file.
func saveResumeState(state *ResumeState) error {
	state.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal resume state: %w", err)
	}
	path := resumeFilePath(state.OutputFile)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write resume state: %w", err)
	}
	return nil
}

// loadResumeState reads an existing resume state from disk.
// Returns nil, nil if the file doesn't exist.
func loadResumeState(outputPath string) (*ResumeState, error) {
	path := resumeFilePath(outputPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read resume state: %w", err)
	}
	var state ResumeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse resume state: %w", err)
	}
	return &state, nil
}

// removeResumeState deletes the sidecar metadata file on successful completion.
func removeResumeState(outputPath string) {
	_ = os.Remove(resumeFilePath(outputPath))
}

// newResumeState creates a fresh resume state for a new download.
func newResumeState(url, outputFile string, totalSize int64, etag, lastModified string, chunks int) *ResumeState {
	now := time.Now()
	chunkSize := totalSize / int64(chunks)
	chunkStates := make([]ChunkState, chunks)
	for i := 0; i < chunks; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if i == chunks-1 {
			end = totalSize - 1
		}
		chunkStates[i] = ChunkState{Start: start, End: end}
	}
	return &ResumeState{
		Version:      1,
		URL:          url,
		OutputFile:   outputFile,
		TotalSize:    totalSize,
		ETag:         etag,
		LastModified: lastModified,
		CreatedAt:    now,
		UpdatedAt:    now,
		Chunks:       chunkStates,
	}
}

// isResumable checks if the saved state matches the current server response.
func (s *ResumeState) isResumable(etag, lastModified string, totalSize int64) bool {
	if s.TotalSize != totalSize {
		return false
	}
	// If we have an ETag, it must match.
	if s.ETag != "" && etag != "" && s.ETag != etag {
		return false
	}
	// If we have Last-Modified, it must match.
	if s.LastModified != "" && lastModified != "" && s.LastModified != lastModified {
		return false
	}
	return true
}

// incompleteChunks returns chunks that are not yet fully downloaded.
func (s *ResumeState) incompleteChunks() []int {
	var indices []int
	for i, c := range s.Chunks {
		if !c.Done {
			indices = append(indices, i)
		}
	}
	return indices
}

// totalDownloaded returns the sum of bytes downloaded across all chunks.
func (s *ResumeState) totalDownloaded() int64 {
	var total int64
	for _, c := range s.Chunks {
		total += c.Downloaded
	}
	return total
}
