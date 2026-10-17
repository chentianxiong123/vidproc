package project

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"vidproc/internal/store"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Migrate(db, ProjectSchema); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateGetUpdateDelete(t *testing.T) {
	db := newTestDB(t)

	id, err := Create(db, "demo", "canvas: 1920x1080", "out.mp4")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id = %d, want > 0", id)
	}

	p, err := Get(db, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.Name != "demo" || p.SpecYAML != "canvas: 1920x1080" || p.OutputPath != "out.mp4" {
		t.Fatalf("got %+v", p)
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", p)
	}

	if err := Update(db, id, "canvas: 1280x720", "out2.mp4"); err != nil {
		t.Fatalf("update: %v", err)
	}
	p2, err := Get(db, id)
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	if p2.SpecYAML != "canvas: 1280x720" || p2.OutputPath != "out2.mp4" {
		t.Fatalf("update not applied: %+v", p2)
	}
	if !p2.UpdatedAt.After(p.UpdatedAt) {
		t.Fatalf("updated_at not bumped: %v -> %v", p.UpdatedAt, p2.UpdatedAt)
	}

	if err := Delete(db, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := Get(db, id); err != ErrNotFound {
		t.Fatalf("get after delete = %v, want ErrNotFound", err)
	}
}

func TestNotFound(t *testing.T) {
	db := newTestDB(t)
	if _, err := Get(db, 999); err != ErrNotFound {
		t.Fatalf("get missing = %v, want ErrNotFound", err)
	}
	if err := Update(db, 999, "x", "y"); err != ErrNotFound {
		t.Fatalf("update missing = %v, want ErrNotFound", err)
	}
	if err := Delete(db, 999); err != ErrNotFound {
		t.Fatalf("delete missing = %v, want ErrNotFound", err)
	}
}

func TestListOrder(t *testing.T) {
	db := newTestDB(t)
	if _, err := Create(db, "a", "s1", "o1"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	id2, err := Create(db, "b", "s2", "o2")
	if err != nil {
		t.Fatal(err)
	}
	if err := Update(db, id2, "s2v2", "o2"); err != nil {
		t.Fatal(err)
	}

	projects, err := List(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("len = %d, want 2", len(projects))
	}
	// 最新更新的 b 在前
	if projects[0].Name != "b" || projects[1].Name != "a" {
		t.Fatalf("order wrong: %+v", projects)
	}
}
