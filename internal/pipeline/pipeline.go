// Package pipeline is the application tracker — the only writable state in
// the whole dashboard, and the only interactive part of the UI.
//
// Deliberately a single JSON file, not SQLite: docs/dashboard-ui.md says a
// static JSON file or a small table "is enough — don't over-engineer this
// piece". The volume is tens of rows edited by one person on one phone.
//
// Writes follow G.A.B.'s save_data() convention: write a temp file, fsync,
// keep the previous file as .bak, then rename into place. A power cut can
// therefore lose the newest edit but never leave a truncated file.
package pipeline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

var ErrNotFound = errors.New("pipeline: item not found")

type Store struct {
	path string

	mu    sync.RWMutex
	items []model.PipelineItem
}

// file is the on-disk shape. Wrapped in an object rather than a bare array so
// fields can be added later without breaking older files.
type file struct {
	Version int                  `json:"version"`
	Items   []model.PipelineItem `json:"items"`
}

const fileVersion = 1

// Open loads the store, creating an empty one if the file does not exist yet.
func Open(path string) (*Store, error) {
	s := &Store{path: path}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("pipeline: cannot create data dir: %w", err)
		}
		return s, s.save() // materialise an empty file so the first write isn't special
	}
	if err != nil {
		return nil, fmt.Errorf("pipeline: cannot read %s: %w", path, err)
	}

	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		// Refuse to start rather than silently overwriting a corrupt file with
		// an empty one — the .bak next to it is likely still good.
		return nil, fmt.Errorf("pipeline: %s is not valid json (%w); .bak may be recoverable", path, err)
	}
	s.items = f.Items
	return s, nil
}

// ============================================================
// READS
// ============================================================

// All returns items sorted newest-updated first within each stage, which is
// the order the phone's JOBS tab renders them.
func (s *Store) All() []model.PipelineItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]model.PipelineItem, len(s.items))
	copy(out, s.items)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// ByStage groups items for the four stage blocks. Every stage is present in
// the map even when empty, so the UI can render "0" instead of omitting it.
func (s *Store) ByStage() map[model.Stage][]model.PipelineItem {
	grouped := make(map[model.Stage][]model.PipelineItem, len(model.Stages))
	for _, stage := range model.Stages {
		grouped[stage] = []model.PipelineItem{}
	}
	for _, item := range s.All() {
		grouped[item.Stage] = append(grouped[item.Stage], item)
	}
	return grouped
}

// ============================================================
// WRITES
// ============================================================

func (s *Store) Add(company, role string, stage model.Stage, note string) (model.PipelineItem, error) {
	if company == "" {
		return model.PipelineItem{}, errors.New("pipeline: company is required")
	}
	if !stage.Valid() {
		return model.PipelineItem{}, fmt.Errorf("pipeline: unknown stage %q", stage)
	}

	now := time.Now()
	item := model.PipelineItem{
		ID: newID(), Company: company, Role: role, Stage: stage,
		Note: note, AddedAt: now, UpdatedAt: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, item)
	if err := s.save(); err != nil {
		s.items = s.items[:len(s.items)-1] // keep memory consistent with disk
		return model.PipelineItem{}, err
	}
	return item, nil
}

// Update applies only the non-nil fields, so the phone can move an item to the
// next stage without resending the whole record.
func (s *Store) Update(id string, company, role, note *string, stage *model.Stage) (model.PipelineItem, error) {
	if stage != nil && !stage.Valid() {
		return model.PipelineItem{}, fmt.Errorf("pipeline: unknown stage %q", *stage)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.items {
		if s.items[i].ID != id {
			continue
		}
		before := s.items[i]
		if company != nil {
			s.items[i].Company = *company
		}
		if role != nil {
			s.items[i].Role = *role
		}
		if note != nil {
			s.items[i].Note = *note
		}
		if stage != nil {
			s.items[i].Stage = *stage
		}
		s.items[i].UpdatedAt = time.Now()

		if err := s.save(); err != nil {
			s.items[i] = before
			return model.PipelineItem{}, err
		}
		return s.items[i], nil
	}
	return model.PipelineItem{}, ErrNotFound
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.items {
		if s.items[i].ID != id {
			continue
		}
		removed := s.items[i]
		s.items = append(s.items[:i], s.items[i+1:]...)
		if err := s.save(); err != nil {
			// Put it back so memory matches what is still on disk.
			s.items = append(s.items, removed)
			return err
		}
		return nil
	}
	return ErrNotFound
}

// ============================================================
// ATOMIC WRITE — matches save_data() in gab_server.py
// ============================================================

// save must be called with s.mu held (or before the store is shared).
func (s *Store) save() error {
	raw, err := json.MarshalIndent(file{Version: fileVersion, Items: s.items}, "", "  ")
	if err != nil {
		return fmt.Errorf("pipeline: cannot encode: %w", err)
	}

	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("pipeline: cannot open temp file: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return fmt.Errorf("pipeline: cannot write temp file: %w", err)
	}
	// fsync before rename: rename is atomic w.r.t. the directory entry, but
	// without this the *contents* may not have reached disk when it happens.
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("pipeline: cannot sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("pipeline: cannot close temp file: %w", err)
	}

	// Keep the previous good file as .bak. Missing on first write, which is
	// not an error.
	if _, err := os.Stat(s.path); err == nil {
		if err := os.Rename(s.path, s.path+".bak"); err != nil {
			return fmt.Errorf("pipeline: cannot rotate backup: %w", err)
		}
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("pipeline: cannot commit write: %w", err)
	}
	return nil
}

// newID is a random 8-byte hex string. Not sequential on purpose — IDs are
// handed to the browser and sequential ones would imply an ordering that the
// UpdatedAt sort does not honour.
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is not recoverable here, and a timestamp ID is
		// still unique enough for a single-user tracker.
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
