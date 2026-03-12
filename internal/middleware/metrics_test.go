package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetrics_PassesResponseThrough(t *testing.T) {
	handler := Metrics("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodPost, "/test", nil))
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestMetrics_DefaultStatus200(t *testing.T) {
	handler := Metrics("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodGet, "/test", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestMetrics_404(t *testing.T) {
	handler := Metrics("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodGet, "/test", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestMetrics_WrapsDifferentMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		handler := Metrics("/test", func(w http.ResponseWriter, r *http.Request) {})
		rr := httptest.NewRecorder()
		handler(rr, httptest.NewRequest(method, "/test", nil))
		if rr.Code != http.StatusOK {
			t.Errorf("method %s: expected 200, got %d", method, rr.Code)
		}
	}
}
