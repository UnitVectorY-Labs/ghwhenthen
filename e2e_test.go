package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/config"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/event"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/rule"
	"github.com/klauspost/compress/zstd"
)

// graphQLRequest captures a decoded GraphQL request for assertions.
type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// mockGraphQLServer tracks requests and returns canned responses for
// addProjectV2ItemById and updateProjectV2ItemFieldValue mutations.
type mockGraphQLServer struct {
	mu       sync.Mutex
	requests []graphQLRequest
}

func (m *mockGraphQLServer) handler(w http.ResponseWriter, r *http.Request) {
	var req graphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Determine which mutation was sent based on request variables.
	if _, ok := req.Variables["contentId"]; ok {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"addProjectV2ItemById": map[string]interface{}{
					"item": map[string]interface{}{
						"id": "PVTI_mock_item_id",
					},
				},
			},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"updateProjectV2ItemFieldValue": map[string]interface{}{
				"projectV2Item": map[string]interface{}{
					"id": "PVTI_mock_item_id",
				},
			},
		},
	})
}

// loadConfigAndEngine loads the example config, overrides the GraphQL
// endpoint, sets the token env var, and returns a ready rule.Engine.
func loadConfigAndEngine(t *testing.T, endpointURL string) *rule.Engine {
	t.Helper()

	t.Setenv("GITHUB_TOKEN", "test-token")

	cfg, err := config.Load("examples/config.yaml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	cfg.GitHub.GraphQLEndpoint = endpointURL

	if err := config.Validate(cfg); err != nil {
		t.Fatalf("config.Validate: %v", err)
	}

	token := os.Getenv(cfg.GitHub.TokenEnvVar)

	engine, err := rule.NewEngine(cfg.Rules, cfg.Constants, cfg.GitHub.GraphQLEndpoint, token)
	if err != nil {
		t.Fatalf("rule.NewEngine: %v", err)
	}
	return engine
}

// TestE2E_PublicPROpened verifies that a public PR "opened" event is routed
// to the public project with the correct GraphQL mutations.
func TestE2E_PublicPROpened(t *testing.T) {
	mock := &mockGraphQLServer{}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	engine := loadConfigAndEngine(t, srv.URL)

	payload := `{
		"repository": {"visibility": "public", "private": false, "full_name": "org/repo"},
		"pull_request": {"node_id": "PR_kwDO123"}
	}`
	attrs := map[string]string{"gh_event": "pull_request", "action": "opened"}
	evt, err := event.BuildEvent([]byte(payload), attrs)
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	matched, ruleName, err := engine.ProcessEvent(context.Background(), evt)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if !matched {
		t.Fatal("expected a rule to match")
	}
	if ruleName != "public-opened-item-to-public-project" {
		t.Errorf("expected rule %q, got %q", "public-opened-item-to-public-project", ruleName)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.requests) != 2 {
		t.Fatalf("expected 2 GraphQL requests, got %d", len(mock.requests))
	}

	// First request: addProjectV2ItemById
	first := mock.requests[0]
	if first.Variables["projectId"] != "PVT_kwDO_PUBLIC" {
		t.Errorf("first request projectId = %v, want PVT_kwDO_PUBLIC", first.Variables["projectId"])
	}
	if first.Variables["contentId"] != "PR_kwDO123" {
		t.Errorf("first request contentId = %v, want PR_kwDO123", first.Variables["contentId"])
	}

	// Second request: updateProjectV2ItemFieldValue
	second := mock.requests[1]
	if second.Variables["projectId"] != "PVT_kwDO_PUBLIC" {
		t.Errorf("second request projectId = %v, want PVT_kwDO_PUBLIC", second.Variables["projectId"])
	}
	if second.Variables["itemId"] != "PVTI_mock_item_id" {
		t.Errorf("second request itemId = %v, want PVTI_mock_item_id", second.Variables["itemId"])
	}
	if second.Variables["fieldId"] != "PVTSSF_PUBLIC_STATUS" {
		t.Errorf("second request fieldId = %v, want PVTSSF_PUBLIC_STATUS", second.Variables["fieldId"])
	}
	if second.Variables["optionId"] != "f75ad846" {
		t.Errorf("second request optionId = %v, want f75ad846", second.Variables["optionId"])
	}
}

// TestE2E_PrivateIssueOpened verifies that a private issue "opened" event
// is routed to the private project.
func TestE2E_PrivateIssueOpened(t *testing.T) {
	mock := &mockGraphQLServer{}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	engine := loadConfigAndEngine(t, srv.URL)

	payload := `{
		"repository": {"visibility": "private", "private": true, "full_name": "org/private-repo"},
		"issue": {"node_id": "I_kwDO456"}
	}`
	attrs := map[string]string{"gh_event": "issues", "action": "opened"}
	evt, err := event.BuildEvent([]byte(payload), attrs)
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	matched, ruleName, err := engine.ProcessEvent(context.Background(), evt)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if !matched {
		t.Fatal("expected a rule to match")
	}
	if ruleName != "private-opened-item-to-private-project" {
		t.Errorf("expected rule %q, got %q", "private-opened-item-to-private-project", ruleName)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.requests) != 2 {
		t.Fatalf("expected 2 GraphQL requests, got %d", len(mock.requests))
	}

	first := mock.requests[0]
	if first.Variables["projectId"] != "PVT_kwDO_PRIVATE" {
		t.Errorf("first request projectId = %v, want PVT_kwDO_PRIVATE", first.Variables["projectId"])
	}
	if first.Variables["contentId"] != "I_kwDO456" {
		t.Errorf("first request contentId = %v, want I_kwDO456", first.Variables["contentId"])
	}

	second := mock.requests[1]
	if second.Variables["projectId"] != "PVT_kwDO_PRIVATE" {
		t.Errorf("second request projectId = %v, want PVT_kwDO_PRIVATE", second.Variables["projectId"])
	}
	if second.Variables["itemId"] != "PVTI_mock_item_id" {
		t.Errorf("second request itemId = %v, want PVTI_mock_item_id", second.Variables["itemId"])
	}
	if second.Variables["fieldId"] != "PVTSSF_PRIVATE_STATUS" {
		t.Errorf("second request fieldId = %v, want PVTSSF_PRIVATE_STATUS", second.Variables["fieldId"])
	}
	if second.Variables["optionId"] != "f75ad846" {
		t.Errorf("second request optionId = %v, want f75ad846", second.Variables["optionId"])
	}
}

// TestE2E_UnrelatedEventNoMatch verifies that an event that doesn't match
// any rule produces no match and no error.
func TestE2E_UnrelatedEventNoMatch(t *testing.T) {
	mock := &mockGraphQLServer{}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	engine := loadConfigAndEngine(t, srv.URL)

	payload := `{"ref": "refs/heads/main", "repository": {"full_name": "org/repo"}}`
	attrs := map[string]string{"gh_event": "push", "action": "completed"}
	evt, err := event.BuildEvent([]byte(payload), attrs)
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	matched, ruleName, err := engine.ProcessEvent(context.Background(), evt)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if matched {
		t.Errorf("expected no match, got rule %q", ruleName)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.requests) != 0 {
		t.Errorf("expected 0 GraphQL requests, got %d", len(mock.requests))
	}
}

// TestE2E_CompressedPayloadGzip verifies that gzip-compressed payloads are
// decompressed and processed correctly.
func TestE2E_CompressedPayloadGzip(t *testing.T) {
	mock := &mockGraphQLServer{}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	engine := loadConfigAndEngine(t, srv.URL)

	payload := `{
		"repository": {"visibility": "public", "private": false, "full_name": "org/repo"},
		"pull_request": {"node_id": "PR_kwDO123"}
	}`

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if _, err := gzw.Write([]byte(payload)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	attrs := map[string]string{
		"gh_event":    "pull_request",
		"action":      "opened",
		"compression": "gzip",
	}
	evt, err := event.BuildEvent(buf.Bytes(), attrs)
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	matched, ruleName, err := engine.ProcessEvent(context.Background(), evt)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if !matched {
		t.Fatal("expected a rule to match")
	}
	if ruleName != "public-opened-item-to-public-project" {
		t.Errorf("expected rule %q, got %q", "public-opened-item-to-public-project", ruleName)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.requests) != 2 {
		t.Fatalf("expected 2 GraphQL requests, got %d", len(mock.requests))
	}
	if mock.requests[0].Variables["contentId"] != "PR_kwDO123" {
		t.Errorf("first request contentId = %v, want PR_kwDO123", mock.requests[0].Variables["contentId"])
	}
}

// TestE2E_CompressedPayloadZstd verifies that zstd-compressed payloads are
// decompressed and processed correctly.
func TestE2E_CompressedPayloadZstd(t *testing.T) {
	mock := &mockGraphQLServer{}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	engine := loadConfigAndEngine(t, srv.URL)

	payload := `{
		"repository": {"visibility": "public", "private": false, "full_name": "org/repo"},
		"pull_request": {"node_id": "PR_kwDO123"}
	}`

	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd.NewWriter: %v", err)
	}
	compressed := encoder.EncodeAll([]byte(payload), nil)
	encoder.Close()

	attrs := map[string]string{
		"gh_event":    "pull_request",
		"action":      "opened",
		"compression": "zstd",
	}
	evt, err := event.BuildEvent(compressed, attrs)
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	matched, ruleName, err := engine.ProcessEvent(context.Background(), evt)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if !matched {
		t.Fatal("expected a rule to match")
	}
	if ruleName != "public-opened-item-to-public-project" {
		t.Errorf("expected rule %q, got %q", "public-opened-item-to-public-project", ruleName)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.requests) != 2 {
		t.Fatalf("expected 2 GraphQL requests, got %d", len(mock.requests))
	}
	if mock.requests[0].Variables["contentId"] != "PR_kwDO123" {
		t.Errorf("first request contentId = %v, want PR_kwDO123", mock.requests[0].Variables["contentId"])
	}
}

// TestE2E_DuplicateItemHandling verifies that even if an item already exists
// in the project, the second step (set status) still executes successfully.
func TestE2E_DuplicateItemHandling(t *testing.T) {
	mock := &mockGraphQLServer{}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	engine := loadConfigAndEngine(t, srv.URL)

	payload := `{
		"repository": {"visibility": "public", "private": false, "full_name": "org/repo"},
		"pull_request": {"node_id": "PR_kwDO_existing"}
	}`
	attrs := map[string]string{"gh_event": "pull_request", "action": "opened"}
	evt, err := event.BuildEvent([]byte(payload), attrs)
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	matched, ruleName, err := engine.ProcessEvent(context.Background(), evt)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if !matched {
		t.Fatal("expected a rule to match")
	}
	if ruleName != "public-opened-item-to-public-project" {
		t.Errorf("expected rule %q, got %q", "public-opened-item-to-public-project", ruleName)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.requests) != 2 {
		t.Fatalf("expected 2 GraphQL requests, got %d", len(mock.requests))
	}

	// The first step returns an existing item ID; the second step should
	// still run and receive that item ID.
	if mock.requests[0].Variables["contentId"] != "PR_kwDO_existing" {
		t.Errorf("first request contentId = %v, want PR_kwDO_existing", mock.requests[0].Variables["contentId"])
	}
	if mock.requests[1].Variables["itemId"] != "PVTI_mock_item_id" {
		t.Errorf("second request itemId = %v, want PVTI_mock_item_id", mock.requests[1].Variables["itemId"])
	}
}
