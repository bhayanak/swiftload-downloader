package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
)

// HistoryEntry represents a single download record persisted across restarts.
type HistoryEntry struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	OutputPath string    `json:"output_path"`
	TotalSize  int64     `json:"total_size"`
	Downloaded int64     `json:"downloaded"`
	Status     string    `json:"status"` // "downloading", "paused", "completed", "failed", "cancelled"
	AddedAt    time.Time `json:"added_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Workers    int       `json:"workers"`
	Parallel   bool      `json:"parallel"`
}

// HistoryStore manages persistent download history.
type HistoryStore struct {
	entries  []HistoryEntry
	mu       sync.Mutex
	filePath string
	saveCh   chan struct{}
	stopCh   chan struct{}
}

// NewHistoryStore creates a history store using the app's storage directory.
func NewHistoryStore(a fyne.App) *HistoryStore {
	dir := a.Storage().RootURI().Path()
	fp := filepath.Join(dir, "history.json")

	hs := &HistoryStore{
		filePath: fp,
		saveCh:   make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
	}
	hs.load()
	go hs.saveLoop()
	return hs
}

// Entries returns a copy of all history entries.
func (hs *HistoryStore) Entries() []HistoryEntry {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	out := make([]HistoryEntry, len(hs.entries))
	copy(out, hs.entries)
	return out
}

// Add appends a new entry and triggers a save.
func (hs *HistoryStore) Add(e HistoryEntry) {
	hs.mu.Lock()
	hs.entries = append(hs.entries, e)
	hs.mu.Unlock()
	hs.requestSave()
}

// Update modifies an existing entry by ID and triggers a save.
func (hs *HistoryStore) Update(id string, fn func(*HistoryEntry)) {
	hs.mu.Lock()
	for i := range hs.entries {
		if hs.entries[i].ID == id {
			fn(&hs.entries[i])
			break
		}
	}
	hs.mu.Unlock()
	hs.requestSave()
}

// Remove deletes an entry by ID and triggers a save.
func (hs *HistoryStore) Remove(id string) {
	hs.mu.Lock()
	for i, e := range hs.entries {
		if e.ID == id {
			hs.entries = append(hs.entries[:i], hs.entries[i+1:]...)
			break
		}
	}
	hs.mu.Unlock()
	hs.requestSave()
}

// Stop shuts down the background save loop and forces a final save.
func (hs *HistoryStore) Stop() {
	close(hs.stopCh)
	hs.save()
}

func (hs *HistoryStore) requestSave() {
	select {
	case hs.saveCh <- struct{}{}:
	default:
	}
}

// saveLoop debounces saves to max once per 2 seconds.
func (hs *HistoryStore) saveLoop() {
	for {
		select {
		case <-hs.stopCh:
			return
		case <-hs.saveCh:
			time.Sleep(2 * time.Second)
			hs.save()
			// Drain any pending signals that accumulated during the sleep.
			select {
			case <-hs.saveCh:
			default:
			}
		}
	}
}

func (hs *HistoryStore) load() {
	data, err := os.ReadFile(hs.filePath)
	if err != nil {
		return // first run, no history file
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	_ = json.Unmarshal(data, &hs.entries)
}

func (hs *HistoryStore) save() {
	hs.mu.Lock()
	data, err := json.MarshalIndent(hs.entries, "", "  ")
	hs.mu.Unlock()
	if err != nil {
		return
	}
	dir := filepath.Dir(hs.filePath)
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(hs.filePath, data, 0644)
}
