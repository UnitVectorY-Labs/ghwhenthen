package health

import (
	"net/http"
	"sync"
)

// Status tracks the health state of the application.
type Status struct {
	mu    sync.RWMutex
	alive bool
	ready bool
}

// NewStatus creates a new Status with alive and ready both set to false.
func NewStatus() *Status {
	return &Status{}
}

// SetAlive marks the process as alive.
func (s *Status) SetAlive(alive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alive = alive
}

// SetReady marks the service as ready to process messages.
func (s *Status) SetReady(ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = ready
}

// IsAlive returns whether the process is alive.
func (s *Status) IsAlive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.alive
}

// IsReady returns whether the service is ready.
func (s *Status) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// Handler returns an http.Handler that serves /livez and /readyz.
func (s *Status) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", s.livezHandler)
	mux.HandleFunc("/readyz", s.readyzHandler)
	return mux
}

func (s *Status) livezHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.IsAlive() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not ok"}`))
	}
}

func (s *Status) readyzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.IsReady() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not ok"}`))
	}
}
