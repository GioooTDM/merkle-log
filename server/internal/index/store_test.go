package index

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func newTestIndexer(t *testing.T) *Indexer {
	t.Helper()

	idx, err := New(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = idx.Close()
	})
	return idx
}

// allows same doc hash
func TestIndexer_GetByDocHash_ReturnsLatest(t *testing.T) {
	idx := newTestIndexer(t)

	docHash := strings.Repeat("a", 64)
	leaf1 := strings.Repeat("1", 64)
	leaf2 := strings.Repeat("2", 64)

	if err := idx.AddEntry("DOC-1", "evt-1", docHash, leaf1, 10); err != nil {
		t.Fatalf("AddEntry #1 error = %v", err)
	}
	if err := idx.AddEntry("DOC-1", "evt-2", docHash, leaf2, 11); err != nil {
		t.Fatalf("AddEntry #2 error = %v", err)
	}

	gotLeaf, gotIndex, err := idx.GetByDocHash(docHash)
	if err != nil {
		t.Fatalf("GetByDocHash() error = %v", err)
	}
	if gotIndex != 11 {
		t.Fatalf("GetByDocHash() index = %d, want 11", gotIndex)
	}
	if gotLeaf != leaf2 {
		t.Fatalf("GetByDocHash() leaf = %q, want %q", gotLeaf, leaf2)
	}
}

func TestIndexer_GetByDocHash_NotFound(t *testing.T) {
	idx := newTestIndexer(t)

	_, _, err := idx.GetByDocHash(strings.Repeat("f", 64))
	if err != sql.ErrNoRows {
		t.Fatalf("GetByDocHash() error = %v, want sql.ErrNoRows", err)
	}
}

func TestIndexer_GetByLeafHash_ReturnsIndex(t *testing.T) {
	idx := newTestIndexer(t)

	leaf := strings.Repeat("1", 64)
	if err := idx.AddEntry("DOC-1", "evt-1", strings.Repeat("a", 64), leaf, 10); err != nil {
		t.Fatalf("AddEntry() error = %v", err)
	}

	got, err := idx.GetByLeafHash(leaf)
	if err != nil {
		t.Fatalf("GetByLeafHash() error = %v", err)
	}
	if got != 10 {
		t.Fatalf("GetByLeafHash() = %d, want 10", got)
	}
}

func TestIndexer_GetByLeafHash_NotFound(t *testing.T) {
	idx := newTestIndexer(t)

	_, err := idx.GetByLeafHash(strings.Repeat("f", 64))
	if err != sql.ErrNoRows {
		t.Fatalf("GetByLeafHash() error = %v, want sql.ErrNoRows", err)
	}
}

func TestIndexer_GetIndexesByDocUID_ReturnsAllAscending(t *testing.T) {
	idx := newTestIndexer(t)

	if err := idx.AddEntry("DOC-1", "evt-1", strings.Repeat("a", 64), strings.Repeat("1", 64), 10); err != nil {
		t.Fatalf("AddEntry #1 error = %v", err)
	}
	if err := idx.AddEntry("DOC-1", "evt-2", strings.Repeat("a", 64), strings.Repeat("2", 64), 11); err != nil {
		t.Fatalf("AddEntry #2 error = %v", err)
	}

	indexes, err := idx.GetIndexesByDocUID("DOC-1")
	if err != nil {
		t.Fatalf("GetIndexesByDocUID() error = %v", err)
	}
	want := []uint64{10, 11}
	if !reflect.DeepEqual(indexes, want) {
		t.Fatalf("GetIndexesByDocUID() = %v, want %v", indexes, want)
	}
}

func TestIndexer_GetIndexesByDocUID_NotFound(t *testing.T) {
	idx := newTestIndexer(t)

	indexes, err := idx.GetIndexesByDocUID("NONEXISTENT")
	if err != nil {
		t.Fatalf("GetIndexesByDocUID() error = %v", err)
	}
	if len(indexes) != 0 {
		t.Fatalf("GetIndexesByDocUID() = %v, want empty", indexes)
	}
}

func TestIndexer_Constraints(t *testing.T) {
	idx := newTestIndexer(t)

	if err := idx.AddEntry("DOC-1", "evt-1", strings.Repeat("a", 64), strings.Repeat("1", 64), 7); err != nil {
		t.Fatalf("AddEntry() seed error = %v", err)
	}

	if err := idx.AddEntry("DOC-2", "evt-2", strings.Repeat("b", 64), strings.Repeat("2", 64), 7); err == nil {
		t.Fatal("expected error on duplicate log_index, got nil")
	}

	if err := idx.AddEntry("DOC-2", "evt-1", strings.Repeat("b", 64), strings.Repeat("3", 64), 8); err == nil {
		t.Fatal("expected error on duplicate event_id, got nil")
	}
}
