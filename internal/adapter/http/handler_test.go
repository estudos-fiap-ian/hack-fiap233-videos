package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hack-fiap233/videos/internal/domain"
)

// --- mock service ---

type mockSvc struct {
	uploadFunc      func(ctx context.Context, title, description string, file io.Reader, filename, userEmail string) (int, error)
	getByIDFunc     func(ctx context.Context, id int) (*domain.Video, error)
	listFunc        func(ctx context.Context) ([]domain.Video, error)
	listByUserFunc  func(ctx context.Context, userEmail string) ([]domain.Video, error)
	createFunc      func(ctx context.Context, title, description string) (*domain.Video, error)
	healthCheckFunc func(ctx context.Context) error
}

func (m *mockSvc) Upload(ctx context.Context, title, description string, file io.Reader, filename, userEmail string) (int, error) {
	return m.uploadFunc(ctx, title, description, file, filename, userEmail)
}
func (m *mockSvc) GetByID(ctx context.Context, id int) (*domain.Video, error) {
	return m.getByIDFunc(ctx, id)
}
func (m *mockSvc) List(ctx context.Context) ([]domain.Video, error) {
	return m.listFunc(ctx)
}
func (m *mockSvc) ListByUser(ctx context.Context, userEmail string) ([]domain.Video, error) {
	return m.listByUserFunc(ctx, userEmail)
}
func (m *mockSvc) Create(ctx context.Context, title, description string) (*domain.Video, error) {
	return m.createFunc(ctx, title, description)
}
func (m *mockSvc) HealthCheck(ctx context.Context) error {
	return m.healthCheckFunc(ctx)
}

// --- helpers ---

func multipartRequest(t *testing.T, fields map[string]string, fileField, filename, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	if fileField != "" {
		fw, _ := mw.CreateFormFile(fileField, filename)
		io.WriteString(fw, content)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/videos/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func decodeJSON(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	json.NewDecoder(body).Decode(&m)
	return m
}

// --- Health ---

func TestHealth_OK(t *testing.T) {
	svc := &mockSvc{healthCheckFunc: func(_ context.Context) error { return nil }}
	rr := httptest.NewRecorder()
	NewHandler(svc).Health(rr, httptest.NewRequest(http.MethodGet, "/videos/health", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := decodeJSON(t, rr.Body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
}

func TestHealth_Unhealthy(t *testing.T) {
	svc := &mockSvc{healthCheckFunc: func(_ context.Context) error { return errors.New("refused") }}
	rr := httptest.NewRecorder()
	NewHandler(svc).Health(rr, httptest.NewRequest(http.MethodGet, "/videos/health", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// --- Upload ---

func TestUpload_Success(t *testing.T) {
	svc := &mockSvc{uploadFunc: func(_ context.Context, _, _ string, _ io.Reader, _, _ string) (int, error) { return 1, nil }}
	rr := httptest.NewRecorder()
	NewHandler(svc).Upload(rr, multipartRequest(t, map[string]string{"title": "Test", "description": "d"}, "video", "test.mp4", "content"))
	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
	body := decodeJSON(t, rr.Body)
	if body["job_id"] == nil {
		t.Error("expected job_id in response")
	}
}

func TestUpload_WrongMethod(t *testing.T) {
	rr := httptest.NewRecorder()
	NewHandler(&mockSvc{}).Upload(rr, httptest.NewRequest(http.MethodGet, "/videos/upload", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestUpload_MissingTitle(t *testing.T) {
	rr := httptest.NewRecorder()
	NewHandler(&mockSvc{}).Upload(rr, multipartRequest(t, map[string]string{"description": "d"}, "video", "f.mp4", "c"))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestUpload_MissingFile(t *testing.T) {
	rr := httptest.NewRecorder()
	NewHandler(&mockSvc{}).Upload(rr, multipartRequest(t, map[string]string{"title": "T"}, "", "", ""))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestUpload_ServiceError(t *testing.T) {
	svc := &mockSvc{uploadFunc: func(_ context.Context, _, _ string, _ io.Reader, _, _ string) (int, error) {
		return 0, errors.New("internal")
	}}
	rr := httptest.NewRecorder()
	NewHandler(svc).Upload(rr, multipartRequest(t, map[string]string{"title": "T"}, "video", "f.mp4", "c"))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// --- Videos: GET list ---

func TestVideos_List(t *testing.T) {
	svc := &mockSvc{listFunc: func(_ context.Context) ([]domain.Video, error) {
		return []domain.Video{{ID: 1}}, nil
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/videos/", nil)
	NewHandler(svc).Videos(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestVideos_ListError(t *testing.T) {
	svc := &mockSvc{listFunc: func(_ context.Context) ([]domain.Video, error) { return nil, errors.New("db") }}
	rr := httptest.NewRecorder()
	NewHandler(svc).Videos(rr, httptest.NewRequest(http.MethodGet, "/videos/", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// --- Videos: GET by ID ---

func TestVideos_GetByID_Found(t *testing.T) {
	svc := &mockSvc{getByIDFunc: func(_ context.Context, id int) (*domain.Video, error) {
		return &domain.Video{ID: id, Title: "Test"}, nil
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/videos/1", nil)
	NewHandler(svc).Videos(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestVideos_GetByID_NotFound(t *testing.T) {
	svc := &mockSvc{getByIDFunc: func(_ context.Context, _ int) (*domain.Video, error) { return nil, nil }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/videos/99", nil)
	NewHandler(svc).Videos(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestVideos_GetByID_Error(t *testing.T) {
	svc := &mockSvc{getByIDFunc: func(_ context.Context, _ int) (*domain.Video, error) { return nil, errors.New("db") }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/videos/1", nil)
	NewHandler(svc).Videos(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// --- Videos: POST create ---

func TestVideos_Create_Success(t *testing.T) {
	svc := &mockSvc{createFunc: func(_ context.Context, title, desc string) (*domain.Video, error) {
		return &domain.Video{ID: 1, Title: title, Description: desc, Status: "pending"}, nil
	}}
	body, _ := json.Marshal(map[string]string{"title": "Test", "description": "d"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/videos/", bytes.NewReader(body))
	NewHandler(svc).Videos(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestVideos_Create_InvalidJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/videos/", strings.NewReader("not json"))
	NewHandler(&mockSvc{}).Videos(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestVideos_Create_ServiceError(t *testing.T) {
	svc := &mockSvc{createFunc: func(_ context.Context, _, _ string) (*domain.Video, error) { return nil, errors.New("db") }}
	body, _ := json.Marshal(map[string]string{"title": "T"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/videos/", bytes.NewReader(body))
	NewHandler(svc).Videos(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestVideos_MethodNotAllowed(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/videos/", nil)
	NewHandler(&mockSvc{}).Videos(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

// --- Videos: GET /videos/me ---

func makeBearer(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, _ := json.Marshal(claims)
	return "Bearer header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestVideos_ListByUser_NoToken(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/videos/me", nil)
	NewHandler(&mockSvc{}).Videos(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestVideos_ListByUser_ServiceError(t *testing.T) {
	svc := &mockSvc{listByUserFunc: func(_ context.Context, _ string) ([]domain.Video, error) {
		return nil, errors.New("db error")
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/videos/me", nil)
	req.Header.Set("Authorization", makeBearer(t, map[string]any{"email": "user@test.com"}))
	NewHandler(svc).Videos(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestVideos_ListByUser_MethodNotAllowed(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/videos/me", nil)
	req.Header.Set("Authorization", makeBearer(t, map[string]any{"email": "user@test.com"}))
	NewHandler(&mockSvc{}).Videos(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

// --- emailFromToken ---

func makeToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, _ := json.Marshal(claims)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestEmailFromToken_Valid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, map[string]any{"email": "test@example.com"}))
	if email := emailFromToken(req); email != "test@example.com" {
		t.Errorf("expected test@example.com, got %s", email)
	}
}

func TestEmailFromToken_NoBearerPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if email := emailFromToken(req); email != "" {
		t.Errorf("expected empty, got %s", email)
	}
}

func TestEmailFromToken_InvalidParts(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer onlyone")
	if email := emailFromToken(req); email != "" {
		t.Errorf("expected empty, got %s", email)
	}
}

func TestEmailFromToken_InvalidBase64(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer header.!!!.sig")
	if email := emailFromToken(req); email != "" {
		t.Errorf("expected empty, got %s", email)
	}
}

func TestEmailFromToken_NoEmailClaim(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, map[string]any{"sub": "user123"}))
	if email := emailFromToken(req); email != "" {
		t.Errorf("expected empty, got %s", email)
	}
}
