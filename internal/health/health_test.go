package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLivezReturns200WhenAlive(t *testing.T) {
	s := NewStatus()
	s.SetAlive(true)

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestLivezReturns503WhenNotAlive(t *testing.T) {
	s := NewStatus()

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
	if rec.Body.String() != `{"status":"not ok"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestReadyzReturns200WhenReady(t *testing.T) {
	s := NewStatus()
	s.SetReady(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestReadyzReturns503WhenNotReady(t *testing.T) {
	s := NewStatus()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
	if rec.Body.String() != `{"status":"not ok"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestDefaultStateIsFalse(t *testing.T) {
	s := NewStatus()

	if s.IsAlive() {
		t.Error("expected alive to be false by default")
	}
	if s.IsReady() {
		t.Error("expected ready to be false by default")
	}
}

func TestSetAliveSetReadyToggle(t *testing.T) {
	s := NewStatus()

	s.SetAlive(true)
	if !s.IsAlive() {
		t.Error("expected alive to be true after SetAlive(true)")
	}

	s.SetAlive(false)
	if s.IsAlive() {
		t.Error("expected alive to be false after SetAlive(false)")
	}

	s.SetReady(true)
	if !s.IsReady() {
		t.Error("expected ready to be true after SetReady(true)")
	}

	s.SetReady(false)
	if s.IsReady() {
		t.Error("expected ready to be false after SetReady(false)")
	}
}

func TestUnknownPathReturns404(t *testing.T) {
	s := NewStatus()

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}
