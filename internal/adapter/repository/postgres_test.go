package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// --- fake row scanner ---

type fakeRow struct {
	vals []any
	err  error
}

func (f *fakeRow) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	for i, d := range dest {
		if i >= len(f.vals) {
			break
		}
		switch v := d.(type) {
		case *int:
			*v = f.vals[i].(int)
		case *string:
			*v = f.vals[i].(string)
		}
	}
	return nil
}

// --- fake rows ---

type fakeRows struct {
	data  [][]any
	index int
	err   error
}

func (f *fakeRows) Next() bool {
	f.index++
	return f.index <= len(f.data)
}

func (f *fakeRows) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	row := f.data[f.index-1]
	for i, d := range dest {
		if i >= len(row) {
			break
		}
		switch v := d.(type) {
		case *int:
			*v = row[i].(int)
		case *string:
			*v = row[i].(string)
		}
	}
	return nil
}

func (f *fakeRows) Close() error { return nil }

// --- mock db ---

type mockDB struct {
	queryRowFunc func(ctx context.Context, query string, args ...any) rowScanner
	queryFunc    func(ctx context.Context, query string, args ...any) (sqlRows, error)
	execFunc     func(ctx context.Context, query string, args ...any) (sql.Result, error)
	pingFunc     func(ctx context.Context) error
}

func (m *mockDB) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return m.queryRowFunc(ctx, query, args...)
}
func (m *mockDB) QueryContext(ctx context.Context, query string, args ...any) (sqlRows, error) {
	return m.queryFunc(ctx, query, args...)
}
func (m *mockDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return m.execFunc(ctx, query, args...)
}
func (m *mockDB) PingContext(ctx context.Context) error {
	return m.pingFunc(ctx)
}

// fakeResult implements sql.Result
type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 1, nil }

func newRepo(db dbQuerier) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// --- Save ---

func TestSave_Success(t *testing.T) {
	db := &mockDB{queryRowFunc: func(_ context.Context, _ string, _ ...any) rowScanner {
		return &fakeRow{vals: []any{1}}
	}}
	id, err := newRepo(db).Save(context.Background(), "title", "desc")
	if err != nil || id != 1 {
		t.Errorf("expected id=1 nil, got %d %v", id, err)
	}
}

func TestSave_Error(t *testing.T) {
	db := &mockDB{queryRowFunc: func(_ context.Context, _ string, _ ...any) rowScanner {
		return &fakeRow{err: errors.New("db error")}
	}}
	_, err := newRepo(db).Save(context.Background(), "title", "desc")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- UpdateS3Key ---

func TestUpdateS3Key_Success(t *testing.T) {
	db := &mockDB{execFunc: func(_ context.Context, _ string, _ ...any) (sql.Result, error) {
		return fakeResult{}, nil
	}}
	if err := newRepo(db).UpdateS3Key(context.Background(), 1, "videos/1/test.mp4"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestUpdateS3Key_Error(t *testing.T) {
	db := &mockDB{execFunc: func(_ context.Context, _ string, _ ...any) (sql.Result, error) {
		return nil, errors.New("db error")
	}}
	if err := newRepo(db).UpdateS3Key(context.Background(), 1, "key"); err == nil {
		t.Fatal("expected error")
	}
}

// --- GetByID ---

func TestGetByID_Found(t *testing.T) {
	db := &mockDB{queryRowFunc: func(_ context.Context, _ string, _ ...any) rowScanner {
		return &fakeRow{vals: []any{1, "Test", "desc", "pending", "s3key", ""}}
	}}
	v, err := newRepo(db).GetByID(context.Background(), 1)
	if err != nil || v == nil || v.ID != 1 || v.Title != "Test" {
		t.Errorf("unexpected result: %+v %v", v, err)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	db := &mockDB{queryRowFunc: func(_ context.Context, _ string, _ ...any) rowScanner {
		return &fakeRow{err: sql.ErrNoRows}
	}}
	v, err := newRepo(db).GetByID(context.Background(), 999)
	if err != nil || v != nil {
		t.Errorf("expected nil video and nil error, got %+v %v", v, err)
	}
}

func TestGetByID_Error(t *testing.T) {
	db := &mockDB{queryRowFunc: func(_ context.Context, _ string, _ ...any) rowScanner {
		return &fakeRow{err: errors.New("db error")}
	}}
	_, err := newRepo(db).GetByID(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- List ---

func TestList_Success(t *testing.T) {
	db := &mockDB{queryFunc: func(_ context.Context, _ string, _ ...any) (sqlRows, error) {
		return &fakeRows{data: [][]any{
			{1, "v1", "d1", "pending", "", ""},
			{2, "v2", "d2", "done", "key", "zip"},
		}}, nil
	}}
	videos, err := newRepo(db).List(context.Background())
	if err != nil || len(videos) != 2 {
		t.Errorf("expected 2 videos, got %d %v", len(videos), err)
	}
}

func TestList_Empty(t *testing.T) {
	db := &mockDB{queryFunc: func(_ context.Context, _ string, _ ...any) (sqlRows, error) {
		return &fakeRows{data: [][]any{}}, nil
	}}
	videos, err := newRepo(db).List(context.Background())
	if err != nil || len(videos) != 0 {
		t.Errorf("expected empty slice, got %d %v", len(videos), err)
	}
}

func TestList_QueryError(t *testing.T) {
	db := &mockDB{queryFunc: func(_ context.Context, _ string, _ ...any) (sqlRows, error) {
		return nil, errors.New("db error")
	}}
	_, err := newRepo(db).List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_ScanError_Skips(t *testing.T) {
	db := &mockDB{queryFunc: func(_ context.Context, _ string, _ ...any) (sqlRows, error) {
		return &fakeRows{data: [][]any{{1, "v1", "d1", "pending", "", ""}}, err: errors.New("scan error")}, nil
	}}
	videos, err := newRepo(db).List(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(videos) != 0 {
		t.Errorf("expected 0 videos (scan errors skipped), got %d", len(videos))
	}
}

// --- Ping ---

func TestPing_Success(t *testing.T) {
	db := &mockDB{pingFunc: func(_ context.Context) error { return nil }}
	if err := newRepo(db).Ping(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestPing_Error(t *testing.T) {
	db := &mockDB{pingFunc: func(_ context.Context) error { return errors.New("connection refused") }}
	if err := newRepo(db).Ping(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

// --- CreateTable ---

func TestCreateTable_Success(t *testing.T) {
	calls := 0
	db := &mockDB{execFunc: func(_ context.Context, _ string, _ ...any) (sql.Result, error) {
		calls++
		return fakeResult{}, nil
	}}
	if err := newRepo(db).CreateTable(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 3 { // CREATE TABLE + 2 migrations
		t.Errorf("expected 3 exec calls, got %d", calls)
	}
}

func TestCreateTable_CreateError(t *testing.T) {
	db := &mockDB{execFunc: func(_ context.Context, _ string, _ ...any) (sql.Result, error) {
		return nil, errors.New("db error")
	}}
	if err := newRepo(db).CreateTable(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateTable_MigrationError(t *testing.T) {
	calls := 0
	db := &mockDB{execFunc: func(_ context.Context, _ string, _ ...any) (sql.Result, error) {
		calls++
		if calls > 1 {
			return nil, errors.New("migration error")
		}
		return fakeResult{}, nil
	}}
	if err := newRepo(db).CreateTable(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
