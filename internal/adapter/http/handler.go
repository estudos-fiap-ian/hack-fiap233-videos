package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hack-fiap233/videos/internal/domain"
)

type VideoService interface {
	Upload(ctx context.Context, title, description string, file io.Reader, filename, userEmail string) (int, error)
	GetByID(ctx context.Context, id int) (*domain.Video, error)
	List(ctx context.Context) ([]domain.Video, error)
	Create(ctx context.Context, title, description string) (*domain.Video, error)
	HealthCheck(ctx context.Context) error
}

type Handler struct {
	svc VideoService
}

func NewHandler(svc VideoService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := h.svc.HealthCheck(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "db": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "videos", "db": "connected"})
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid multipart form"})
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	if title == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "title is required"})
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "video file is required"})
		return
	}
	defer file.Close()

	videoID, err := h.svc.Upload(r.Context(), title, description, file, header.Filename, emailFromToken(r))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id":  videoID,
		"status":  "pending",
		"message": "video upload received, processing will start shortly",
	})
}

func (h *Handler) Videos(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	idStr := r.URL.Path[len("/videos/"):]
	if idStr != "" {
		if videoID, err := strconv.Atoi(idStr); err == nil {
			h.getByID(w, r, videoID)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) getByID(w http.ResponseWriter, r *http.Request, id int) {
	v, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if v == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "video not found"})
		return
	}
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	videos, err := h.svc.List(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(videos)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	v, err := h.svc.Create(r.Context(), body.Title, body.Description)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(v)
}

// emailFromToken decodes the JWT payload (no verification — API Gateway already validated it)
// and extracts the email claim.
func emailFromToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	parts := strings.Split(auth[7:], ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	email, _ := claims["email"].(string)
	return email
}
