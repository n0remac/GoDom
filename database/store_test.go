package database

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSQLiteStorePutGetDeleteListQuery(t *testing.T) {
	s, err := NewSQLiteStoreFromDSN(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	doc := map[string]any{"name": "Alice", "age": 30}
	b, _ := json.Marshal(doc)

	if err := s.Put(ctx, "u1", b); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatalf("expected document")
	}

	ids, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 id, got %d", len(ids))
	}

	matches, err := s.Query(ctx, "Alice")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected query to match")
	}

	if err := s.Delete(ctx, "u1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got2, err := s.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got2 != nil {
		t.Fatalf("expected nil after delete")
	}
}
