package application

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/hack-fiap233/videos/internal/domain"
)

// --- mocks ---

type mockRepo struct {
	saveFunc        func(ctx context.Context, title, description string) (int, error)
	updateS3KeyFunc func(ctx context.Context, id int, s3Key string) error
	getByIDFunc     func(ctx context.Context, id int) (*domain.Video, error)
	listFunc        func(ctx context.Context) ([]domain.Video, error)
	pingFunc        func(ctx context.Context) error
}

func (m *mockRepo) Save(ctx context.Context, title, description string) (int, error) {
	return m.saveFunc(ctx, title, description)
}
func (m *mockRepo) UpdateS3Key(ctx context.Context, id int, s3Key string) error {
	return m.updateS3KeyFunc(ctx, id, s3Key)
}
func (m *mockRepo) GetByID(ctx context.Context, id int) (*domain.Video, error) {
	return m.getByIDFunc(ctx, id)
}
func (m *mockRepo) List(ctx context.Context) ([]domain.Video, error) {
	return m.listFunc(ctx)
}
func (m *mockRepo) Ping(ctx context.Context) error {
	return m.pingFunc(ctx)
}

type mockStorage struct {
	uploadFunc func(ctx context.Context, key string, body io.Reader) error
}

func (m *mockStorage) Upload(ctx context.Context, key string, body io.Reader) error {
	return m.uploadFunc(ctx, key, body)
}

type mockPublisher struct {
	publishFunc func(ctx context.Context, event domain.VideoEvent) error
}

func (m *mockPublisher) Publish(ctx context.Context, event domain.VideoEvent) error {
	return m.publishFunc(ctx, event)
}

// --- Upload ---

func TestUpload_Success(t *testing.T) {
	repo := &mockRepo{
		saveFunc:        func(_ context.Context, _, _ string) (int, error) { return 1, nil },
		updateS3KeyFunc: func(_ context.Context, _ int, _ string) error { return nil },
	}
	stor := &mockStorage{uploadFunc: func(_ context.Context, _ string, _ io.Reader) error { return nil }}
	pub := &mockPublisher{publishFunc: func(_ context.Context, _ domain.VideoEvent) error { return nil }}

	id, err := NewVideoService(repo, stor, pub).Upload(context.Background(), "title", "desc", strings.NewReader("data"), "video.mp4", "user@test.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id != 1 {
		t.Errorf("expected id=1, got %d", id)
	}
}

func TestUpload_S3KeyFormat(t *testing.T) {
	var capturedKey string
	repo := &mockRepo{
		saveFunc:        func(_ context.Context, _, _ string) (int, error) { return 42, nil },
		updateS3KeyFunc: func(_ context.Context, _ int, key string) error { return nil },
	}
	stor := &mockStorage{uploadFunc: func(_ context.Context, key string, _ io.Reader) error { capturedKey = key; return nil }}
	pub := &mockPublisher{publishFunc: func(_ context.Context, _ domain.VideoEvent) error { return nil }}

	NewVideoService(repo, stor, pub).Upload(context.Background(), "t", "d", strings.NewReader(""), "clip.mp4", "")
	if capturedKey != "videos/42/clip.mp4" {
		t.Errorf("unexpected s3 key: %s", capturedKey)
	}
}

func TestUpload_SaveFails(t *testing.T) {
	repo := &mockRepo{saveFunc: func(_ context.Context, _, _ string) (int, error) { return 0, errors.New("db error") }}
	_, err := NewVideoService(repo, &mockStorage{}, &mockPublisher{}).Upload(context.Background(), "t", "d", strings.NewReader(""), "v.mp4", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpload_S3Fails(t *testing.T) {
	repo := &mockRepo{saveFunc: func(_ context.Context, _, _ string) (int, error) { return 1, nil }}
	stor := &mockStorage{uploadFunc: func(_ context.Context, _ string, _ io.Reader) error { return errors.New("s3 error") }}
	_, err := NewVideoService(repo, stor, &mockPublisher{}).Upload(context.Background(), "t", "d", strings.NewReader(""), "v.mp4", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpload_UpdateS3KeyFails_ContinuesWithoutError(t *testing.T) {
	repo := &mockRepo{
		saveFunc:        func(_ context.Context, _, _ string) (int, error) { return 1, nil },
		updateS3KeyFunc: func(_ context.Context, _ int, _ string) error { return errors.New("update error") },
	}
	stor := &mockStorage{uploadFunc: func(_ context.Context, _ string, _ io.Reader) error { return nil }}
	pub := &mockPublisher{publishFunc: func(_ context.Context, _ domain.VideoEvent) error { return nil }}

	_, err := NewVideoService(repo, stor, pub).Upload(context.Background(), "t", "d", strings.NewReader(""), "v.mp4", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestUpload_PublishFails_ContinuesWithoutError(t *testing.T) {
	repo := &mockRepo{
		saveFunc:        func(_ context.Context, _, _ string) (int, error) { return 1, nil },
		updateS3KeyFunc: func(_ context.Context, _ int, _ string) error { return nil },
	}
	stor := &mockStorage{uploadFunc: func(_ context.Context, _ string, _ io.Reader) error { return nil }}
	pub := &mockPublisher{publishFunc: func(_ context.Context, _ domain.VideoEvent) error { return errors.New("sns error") }}

	_, err := NewVideoService(repo, stor, pub).Upload(context.Background(), "t", "d", strings.NewReader(""), "v.mp4", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestUpload_EventPayload(t *testing.T) {
	var got domain.VideoEvent
	repo := &mockRepo{
		saveFunc:        func(_ context.Context, _, _ string) (int, error) { return 7, nil },
		updateS3KeyFunc: func(_ context.Context, _ int, _ string) error { return nil },
	}
	stor := &mockStorage{uploadFunc: func(_ context.Context, _ string, _ io.Reader) error { return nil }}
	pub := &mockPublisher{publishFunc: func(_ context.Context, e domain.VideoEvent) error { got = e; return nil }}

	NewVideoService(repo, stor, pub).Upload(context.Background(), "My Title", "d", strings.NewReader(""), "vid.mp4", "user@x.com")
	if got.VideoID != 7 || got.Title != "My Title" || got.UserEmail != "user@x.com" {
		t.Errorf("unexpected event: %+v", got)
	}
}

// --- GetByID ---

func TestGetByID_Found(t *testing.T) {
	repo := &mockRepo{getByIDFunc: func(_ context.Context, _ int) (*domain.Video, error) {
		return &domain.Video{ID: 1, Title: "Test"}, nil
	}}
	v, err := NewVideoService(repo, nil, nil).GetByID(context.Background(), 1)
	if err != nil || v == nil || v.ID != 1 {
		t.Errorf("unexpected result: v=%v err=%v", v, err)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := &mockRepo{getByIDFunc: func(_ context.Context, _ int) (*domain.Video, error) { return nil, nil }}
	v, err := NewVideoService(repo, nil, nil).GetByID(context.Background(), 999)
	if err != nil || v != nil {
		t.Errorf("expected nil video and nil error, got v=%v err=%v", v, err)
	}
}

func TestGetByID_Error(t *testing.T) {
	repo := &mockRepo{getByIDFunc: func(_ context.Context, _ int) (*domain.Video, error) { return nil, errors.New("db error") }}
	_, err := NewVideoService(repo, nil, nil).GetByID(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- List ---

func TestList_Success(t *testing.T) {
	repo := &mockRepo{listFunc: func(_ context.Context) ([]domain.Video, error) {
		return []domain.Video{{ID: 1}, {ID: 2}}, nil
	}}
	result, err := NewVideoService(repo, nil, nil).List(context.Background())
	if err != nil || len(result) != 2 {
		t.Errorf("expected 2 videos and no error, got %d %v", len(result), err)
	}
}

func TestList_Error(t *testing.T) {
	repo := &mockRepo{listFunc: func(_ context.Context) ([]domain.Video, error) { return nil, errors.New("db error") }}
	_, err := NewVideoService(repo, nil, nil).List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Create ---

func TestCreate_Success(t *testing.T) {
	repo := &mockRepo{saveFunc: func(_ context.Context, _, _ string) (int, error) { return 42, nil }}
	v, err := NewVideoService(repo, nil, nil).Create(context.Background(), "My Video", "desc")
	if err != nil || v.ID != 42 || v.Title != "My Video" || v.Status != "pending" {
		t.Errorf("unexpected result: %+v %v", v, err)
	}
}

func TestCreate_Error(t *testing.T) {
	repo := &mockRepo{saveFunc: func(_ context.Context, _, _ string) (int, error) { return 0, errors.New("db error") }}
	_, err := NewVideoService(repo, nil, nil).Create(context.Background(), "t", "d")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- HealthCheck ---

func TestHealthCheck_Success(t *testing.T) {
	repo := &mockRepo{pingFunc: func(_ context.Context) error { return nil }}
	if err := NewVideoService(repo, nil, nil).HealthCheck(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestHealthCheck_Error(t *testing.T) {
	repo := &mockRepo{pingFunc: func(_ context.Context) error { return errors.New("connection refused") }}
	if err := NewVideoService(repo, nil, nil).HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
