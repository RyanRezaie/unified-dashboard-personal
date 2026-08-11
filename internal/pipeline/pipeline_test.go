package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pipeline.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, path
}

func TestAddPersistsAcrossReopen(t *testing.T) {
	s, path := newStore(t)
	if _, err := s.Add("TI", "analog intern", model.StageInterview, ""); err != nil {
		t.Fatalf("Add: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	items := reopened.All()
	if len(items) != 1 || items[0].Company != "TI" {
		t.Fatalf("expected the item to survive a reopen, got %+v", items)
	}
}

func TestAddRejectsBadInput(t *testing.T) {
	s, _ := newStore(t)

	if _, err := s.Add("", "role", model.StageEmailed, ""); err == nil {
		t.Error("expected an error for an empty company")
	}
	if _, err := s.Add("TI", "role", model.Stage("hired"), ""); err == nil {
		t.Error("expected an error for an unknown stage")
	}
	if got := len(s.All()); got != 0 {
		t.Errorf("rejected writes must not land, got %d items", got)
	}
}

func TestUpdateOnlyTouchesProvidedFields(t *testing.T) {
	s, _ := newStore(t)
	item, _ := s.Add("TI", "analog intern", model.StageEmailed, "referred by Sam")

	// The phone's tap-to-advance sends only the stage.
	stage := model.StageApplied
	updated, err := s.Update(item.ID, nil, nil, nil, &stage)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Stage != model.StageApplied {
		t.Errorf("stage = %q, want applied", updated.Stage)
	}
	if updated.Company != "TI" || updated.Role != "analog intern" || updated.Note != "referred by Sam" {
		t.Errorf("untouched fields were clobbered: %+v", updated)
	}
	if !updated.UpdatedAt.After(updated.AddedAt) && updated.UpdatedAt.Before(updated.AddedAt) {
		t.Error("UpdatedAt should move forward on a write")
	}
}

func TestUpdateAndDeleteMissingID(t *testing.T) {
	s, _ := newStore(t)

	if _, err := s.Update("nope", nil, nil, nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update of a missing id: got %v, want ErrNotFound", err)
	}
	if err := s.Delete("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete of a missing id: got %v, want ErrNotFound", err)
	}
}

func TestDeleteRemovesItem(t *testing.T) {
	s, _ := newStore(t)
	keep, _ := s.Add("Sandia", "FPGA", model.StageInterview, "")
	drop, _ := s.Add("Chevron", "", model.StageEmailed, "")

	if err := s.Delete(drop.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	items := s.All()
	if len(items) != 1 || items[0].ID != keep.ID {
		t.Fatalf("expected only the kept item, got %+v", items)
	}
}

// The atomic-write convention is inherited from G.A.B.'s save_data(): the
// previous good file must survive as .bak, and no temp file may be left behind.
func TestSaveKeepsBackupAndCleansUp(t *testing.T) {
	s, path := newStore(t)
	if _, err := s.Add("first", "", model.StageEmailed, ""); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add("second", "", model.StageEmailed, ""); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("expected a .bak alongside the store: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp file should not survive a completed write: %v", err)
	}

	// The backup must be the state *before* the last write.
	backup, err := Open(path + ".bak")
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	if got := len(backup.All()); got != 1 {
		t.Errorf("backup holds %d items, want the 1 from before the last write", got)
	}
}

func TestOpenRefusesCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipeline.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Refusing to start beats silently replacing Ryan's data with an empty file.
	if _, err := Open(path); err == nil {
		t.Fatal("expected Open to refuse a corrupt file")
	}
}

func TestByStageAlwaysHasEveryStage(t *testing.T) {
	s, _ := newStore(t)
	s.Add("TI", "", model.StageInterview, "")

	grouped := s.ByStage()
	for _, stage := range model.Stages {
		if _, ok := grouped[stage]; !ok {
			t.Errorf("stage %q missing — the UI renders a 0 for it, not nothing", stage)
		}
	}
	if len(grouped[model.StageInterview]) != 1 {
		t.Errorf("interview holds %d, want 1", len(grouped[model.StageInterview]))
	}
}
