package index

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

	if err := idx.AddEntry(Entry{LogIndex: 10, DocID: "DOC-1", EventID: "evt-1", DocHash: docHash, LeafHash: leaf1, IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:00:00Z"}); err != nil {
		t.Fatalf("AddEntry #1 error = %v", err)
	}
	if err := idx.AddEntry(Entry{LogIndex: 11, DocID: "DOC-1", EventID: "evt-2", DocHash: docHash, LeafHash: leaf2, IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:01:00Z"}); err != nil {
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
	if err := idx.AddEntry(Entry{LogIndex: 10, DocID: "DOC-1", EventID: "evt-1", DocHash: strings.Repeat("a", 64), LeafHash: leaf, IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:00:00Z"}); err != nil {
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

func TestIndexer_GetIndexesByDocID_ReturnsAllAscending(t *testing.T) {
	idx := newTestIndexer(t)

	if err := idx.AddEntry(Entry{LogIndex: 10, DocID: "DOC-1", EventID: "evt-1", DocHash: strings.Repeat("a", 64), LeafHash: strings.Repeat("1", 64), IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:00:00Z"}); err != nil {
		t.Fatalf("AddEntry #1 error = %v", err)
	}
	if err := idx.AddEntry(Entry{LogIndex: 11, DocID: "DOC-1", EventID: "evt-2", DocHash: strings.Repeat("a", 64), LeafHash: strings.Repeat("2", 64), IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:01:00Z"}); err != nil {
		t.Fatalf("AddEntry #2 error = %v", err)
	}

	indexes, err := idx.GetIndexesByDocID("DOC-1")
	if err != nil {
		t.Fatalf("GetIndexesByDocID() error = %v", err)
	}
	want := []uint64{10, 11}
	if !reflect.DeepEqual(indexes, want) {
		t.Fatalf("GetIndexesByDocID() = %v, want %v", indexes, want)
	}
}

func TestIndexer_GetIndexesByDocID_NotFound(t *testing.T) {
	idx := newTestIndexer(t)

	indexes, err := idx.GetIndexesByDocID("NONEXISTENT")
	if err != nil {
		t.Fatalf("GetIndexesByDocID() error = %v", err)
	}
	if len(indexes) != 0 {
		t.Fatalf("GetIndexesByDocID() = %v, want empty", indexes)
	}
}

func TestIndexer_GetLatestIndexByDocID(t *testing.T) {
	idx := newTestIndexer(t)

	if err := idx.AddEntry(Entry{LogIndex: 10, DocID: "DOC-1", EventID: "evt-1", DocHash: strings.Repeat("a", 64), LeafHash: strings.Repeat("1", 64), IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:00:00Z"}); err != nil {
		t.Fatalf("AddEntry #1 error = %v", err)
	}
	if err := idx.AddEntry(Entry{LogIndex: 12, DocID: "DOC-1", EventID: "evt-2", DocHash: strings.Repeat("b", 64), LeafHash: strings.Repeat("2", 64), IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:01:00Z"}); err != nil {
		t.Fatalf("AddEntry #2 error = %v", err)
	}
	if err := idx.AddEntry(Entry{LogIndex: 11, DocID: "DOC-2", EventID: "evt-3", DocHash: strings.Repeat("c", 64), LeafHash: strings.Repeat("3", 64), IssuerEntityID: "IPA:C_MILANO", RecordedAt: "2026-01-01T10:02:00Z"}); err != nil {
		t.Fatalf("AddEntry #3 error = %v", err)
	}

	got, found, err := idx.GetLatestIndexByDocID("DOC-1")
	if err != nil {
		t.Fatalf("GetLatestIndexByDocID() error = %v", err)
	}
	if !found {
		t.Fatal("GetLatestIndexByDocID() found = false, want true")
	}
	if got != 12 {
		t.Fatalf("GetLatestIndexByDocID() = %d, want 12", got)
	}
}

func TestIndexer_GetLatestIndexByDocID_NotFound(t *testing.T) {
	idx := newTestIndexer(t)

	got, found, err := idx.GetLatestIndexByDocID("NONEXISTENT")
	if err != nil {
		t.Fatalf("GetLatestIndexByDocID() error = %v", err)
	}
	if found {
		t.Fatalf("GetLatestIndexByDocID() found = true with index %d, want false", got)
	}
}

func TestIndexer_Constraints(t *testing.T) {
	idx := newTestIndexer(t)

	if err := idx.AddEntry(Entry{LogIndex: 7, DocID: "DOC-1", EventID: "evt-1", DocHash: strings.Repeat("a", 64), LeafHash: strings.Repeat("1", 64), IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:00:00Z"}); err != nil {
		t.Fatalf("AddEntry() seed error = %v", err)
	}

	if err := idx.AddEntry(Entry{LogIndex: 7, DocID: "DOC-2", EventID: "evt-2", DocHash: strings.Repeat("b", 64), LeafHash: strings.Repeat("2", 64), IssuerEntityID: "IPA:C_MILANO", RecordedAt: "2026-01-01T10:01:00Z"}); err == nil {
		t.Fatal("expected error on duplicate log_index, got nil")
	}

	if err := idx.AddEntry(Entry{LogIndex: 8, DocID: "DOC-2", EventID: "evt-1", DocHash: strings.Repeat("b", 64), LeafHash: strings.Repeat("3", 64), IssuerEntityID: "IPA:C_MILANO", RecordedAt: "2026-01-01T10:02:00Z"}); err == nil {
		t.Fatal("expected error on duplicate event_id, got nil")
	}
}

func TestIndexer_SearchIndexes(t *testing.T) {
	idx := newTestIndexer(t)

	mustAdd := func(entry Entry) {
		t.Helper()
		if err := idx.AddEntry(entry); err != nil {
			t.Fatalf("AddEntry(%+v) error = %v", entry, err)
		}
	}

	mustAdd(Entry{LogIndex: 10, DocID: "DOC-1", EventID: "evt-1", DocHash: strings.Repeat("a", 64), LeafHash: strings.Repeat("1", 64), IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-01T10:00:00Z"})
	mustAdd(Entry{LogIndex: 11, DocID: "DOC-2", EventID: "evt-2", DocHash: strings.Repeat("b", 64), LeafHash: strings.Repeat("2", 64), IssuerEntityID: "IPA:C_MILANO", RecordedAt: "2026-01-02T10:00:00Z"})
	mustAdd(Entry{LogIndex: 12, DocID: "DOC-1", EventID: "evt-3", DocHash: strings.Repeat("c", 64), LeafHash: strings.Repeat("3", 64), IssuerEntityID: "IPA:C_ROMA", RecordedAt: "2026-01-03T10:00:00Z"})

	t.Run("all desc paginated", func(t *testing.T) {
		got, err := idx.SearchIndexes(SearchParams{
			Order:  "desc",
			Limit:  2,
			Offset: 0,
		})
		if err != nil {
			t.Fatalf("SearchIndexes() error = %v", err)
		}
		if got.TotalCount != 3 {
			t.Fatalf("TotalCount = %d, want 3", got.TotalCount)
		}
		want := []uint64{12, 11}
		if !reflect.DeepEqual(got.Indexes, want) {
			t.Fatalf("Indexes = %v, want %v", got.Indexes, want)
		}
	})

	t.Run("doc and issuer filters", func(t *testing.T) {
		got, err := idx.SearchIndexes(SearchParams{
			DocID:          "DOC-1",
			IssuerEntityID: "IPA:C_ROMA",
			Order:          "asc",
		})
		if err != nil {
			t.Fatalf("SearchIndexes() error = %v", err)
		}
		if got.TotalCount != 2 {
			t.Fatalf("TotalCount = %d, want 2", got.TotalCount)
		}
		want := []uint64{10, 12}
		if !reflect.DeepEqual(got.Indexes, want) {
			t.Fatalf("Indexes = %v, want %v", got.Indexes, want)
		}
	})

	t.Run("date filter", func(t *testing.T) {
		got, err := idx.SearchIndexes(SearchParams{
			FromInclusive: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			ToExclusive:   time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
			Order:         "desc",
		})
		if err != nil {
			t.Fatalf("SearchIndexes() error = %v", err)
		}
		if got.TotalCount != 2 {
			t.Fatalf("TotalCount = %d, want 2", got.TotalCount)
		}
		want := []uint64{12, 11}
		if !reflect.DeepEqual(got.Indexes, want) {
			t.Fatalf("Indexes = %v, want %v", got.Indexes, want)
		}
	})
}
