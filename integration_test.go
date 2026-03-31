package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/config"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/consumer"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/health"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/rule"
)

// --- Mock PubSub implementations for integration tests ---

// integrationMockMessage implements consumer.PubSubMessage.
type integrationMockMessage struct {
	mu         sync.Mutex
	id         string
	data       []byte
	attributes map[string]string
	acked      bool
	nacked     bool
}

func (m *integrationMockMessage) ID() string                    { return m.id }
func (m *integrationMockMessage) Data() []byte                  { return m.data }
func (m *integrationMockMessage) Attributes() map[string]string { return m.attributes }

func (m *integrationMockMessage) Ack() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acked = true
}

func (m *integrationMockMessage) Nack() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nacked = true
}

// integrationMockSubscription implements consumer.PubSubSubscription.
type integrationMockSubscription struct {
	msg consumer.PubSubMessage
}

func (ms *integrationMockSubscription) Receive(ctx context.Context, f func(ctx context.Context, msg consumer.PubSubMessage)) error {
	f(ctx, ms.msg)
	return nil
}

// integrationMockClient implements consumer.PubSubClient.
type integrationMockClient struct {
	sub consumer.PubSubSubscription
}

func (mc *integrationMockClient) Subscription(id string) consumer.PubSubSubscription {
	return mc.sub
}

func (mc *integrationMockClient) Close() error {
	return nil
}

// integrationMockFactory returns a consumer.WithClientFactory option using the mock.
func integrationMockFactory(msg consumer.PubSubMessage) func(ctx context.Context, projectID string) (consumer.PubSubClient, error) {
	return func(ctx context.Context, projectID string) (consumer.PubSubClient, error) {
		return &integrationMockClient{
			sub: &integrationMockSubscription{msg: msg},
		}, nil
	}
}

// trackingGraphQLServer records all GraphQL requests and returns canned responses.
type trackingGraphQLServer struct {
	mu       sync.Mutex
	requests []graphQLRequest
}

func (s *trackingGraphQLServer) handler(w http.ResponseWriter, r *http.Request) {
	// Verify request headers
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req graphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Return appropriate response based on mutation type
	if _, ok := req.Variables["contentId"]; ok {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"addProjectV2ItemById": map[string]interface{}{
					"item": map[string]interface{}{
						"id": "PVTI_integration_item",
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
					"id": "PVTI_integration_item",
				},
			},
		},
	})
}

func (s *trackingGraphQLServer) getRequests() []graphQLRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]graphQLRequest, len(s.requests))
	copy(cp, s.requests)
	return cp
}

// --- Integration helper ---

// setupIntegrationConsumer loads the example config, creates a mock GraphQL server,
// wires up a mock Pub/Sub message, and returns the consumer + message + server for assertions.
func setupIntegrationConsumer(t *testing.T, payload []byte, attrs map[string]string, onFailure string) (*consumer.Consumer, *integrationMockMessage, *trackingGraphQLServer) {
	t.Helper()

	t.Setenv("GITHUB_TOKEN", "test-integration-token")

	cfg, err := config.Load("examples/config.yaml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	gql := &trackingGraphQLServer{}
	srv := httptest.NewServer(http.HandlerFunc(gql.handler))
	t.Cleanup(srv.Close)

	cfg.GitHub.GraphQLEndpoint = srv.URL

	if err := config.Validate(cfg); err != nil {
		t.Fatalf("config.Validate: %v", err)
	}

	engine, err := rule.NewEngine(cfg.Rules, cfg.Constants, cfg.GitHub.GraphQLEndpoint, "test-integration-token")
	if err != nil {
		t.Fatalf("rule.NewEngine: %v", err)
	}

	msg := &integrationMockMessage{
		id:         "integration-msg-1",
		data:       payload,
		attributes: attrs,
	}

	h := health.NewStatus()
	h.SetAlive(true)

	failure := onFailure
	if failure == "" {
		failure = cfg.Behavior.OnFailure
	}

	c := consumer.New(
		cfg.PubSub.ProjectID,
		cfg.PubSub.SubscriptionID,
		failure,
		engine,
		h,
		consumer.WithClientFactory(integrationMockFactory(msg)),
	)

	return c, msg, gql
}

// --- Integration Tests ---

// TestIntegration_FullFlow_PublicPROpened exercises the complete path:
// mock Pub/Sub message → consumer → config-driven rule matching → two-step
// GraphQL execution with output chaining → message ack.
func TestIntegration_FullFlow_PublicPROpened(t *testing.T) {
	payload := []byte(`{
		"repository": {"visibility": "public", "private": false, "full_name": "org/repo"},
		"pull_request": {"node_id": "PR_integration_123"}
	}`)
	attrs := map[string]string{
		"gh_event":    "pull_request",
		"action":      "opened",
		"gh_delivery": "delivery-1",
		"repository":  "org/repo",
	}

	c, msg, gql := setupIntegrationConsumer(t, payload, attrs, "")

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("consumer.Run: %v", err)
	}

	// Message should be acked on success
	msg.mu.Lock()
	acked := msg.acked
	nacked := msg.nacked
	msg.mu.Unlock()

	if !acked {
		t.Error("expected message to be acked")
	}
	if nacked {
		t.Error("expected message not to be nacked")
	}

	// Verify two GraphQL mutations were sent
	requests := gql.getRequests()
	if len(requests) != 2 {
		t.Fatalf("expected 2 GraphQL requests, got %d", len(requests))
	}

	// First mutation: addProjectV2ItemById
	first := requests[0]
	if first.Variables["projectId"] != "PVT_kwDO_PUBLIC" {
		t.Errorf("first request projectId = %v, want PVT_kwDO_PUBLIC", first.Variables["projectId"])
	}
	if first.Variables["contentId"] != "PR_integration_123" {
		t.Errorf("first request contentId = %v, want PR_integration_123", first.Variables["contentId"])
	}

	// Second mutation: updateProjectV2ItemFieldValue using output from first
	second := requests[1]
	if second.Variables["projectId"] != "PVT_kwDO_PUBLIC" {
		t.Errorf("second request projectId = %v, want PVT_kwDO_PUBLIC", second.Variables["projectId"])
	}
	if second.Variables["itemId"] != "PVTI_integration_item" {
		t.Errorf("second request itemId = %v, want PVTI_integration_item (chained from step 1)", second.Variables["itemId"])
	}
	if second.Variables["fieldId"] != "PVTSSF_PUBLIC_STATUS" {
		t.Errorf("second request fieldId = %v, want PVTSSF_PUBLIC_STATUS", second.Variables["fieldId"])
	}
	if second.Variables["optionId"] != "f75ad846" {
		t.Errorf("second request optionId = %v, want f75ad846", second.Variables["optionId"])
	}
}

// TestIntegration_FullFlow_PrivateIssueOpened verifies private issue routing
// through the full consumer → rule → GraphQL pipeline.
func TestIntegration_FullFlow_PrivateIssueOpened(t *testing.T) {
	payload := []byte(`{
		"repository": {"visibility": "private", "private": true, "full_name": "org/private-repo"},
		"issue": {"node_id": "I_integration_456"}
	}`)
	attrs := map[string]string{
		"gh_event":    "issues",
		"action":      "opened",
		"gh_delivery": "delivery-2",
		"repository":  "org/private-repo",
	}

	c, msg, gql := setupIntegrationConsumer(t, payload, attrs, "")

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("consumer.Run: %v", err)
	}

	msg.mu.Lock()
	acked := msg.acked
	msg.mu.Unlock()

	if !acked {
		t.Error("expected message to be acked")
	}

	requests := gql.getRequests()
	if len(requests) != 2 {
		t.Fatalf("expected 2 GraphQL requests, got %d", len(requests))
	}

	if requests[0].Variables["projectId"] != "PVT_kwDO_PRIVATE" {
		t.Errorf("first request projectId = %v, want PVT_kwDO_PRIVATE", requests[0].Variables["projectId"])
	}
	if requests[0].Variables["contentId"] != "I_integration_456" {
		t.Errorf("first request contentId = %v, want I_integration_456", requests[0].Variables["contentId"])
	}
	if requests[1].Variables["itemId"] != "PVTI_integration_item" {
		t.Errorf("second request itemId = %v, want PVTI_integration_item", requests[1].Variables["itemId"])
	}
	if requests[1].Variables["fieldId"] != "PVTSSF_PRIVATE_STATUS" {
		t.Errorf("second request fieldId = %v, want PVTSSF_PRIVATE_STATUS", requests[1].Variables["fieldId"])
	}
}

// TestIntegration_NoMatch_MessageAcked verifies that unmatched events flow
// through the consumer and result in message ack (no error, no GraphQL calls).
func TestIntegration_NoMatch_MessageAcked(t *testing.T) {
	payload := []byte(`{"ref": "refs/heads/main", "repository": {"full_name": "org/repo"}}`)
	attrs := map[string]string{
		"gh_event":    "push",
		"action":      "completed",
		"gh_delivery": "delivery-3",
		"repository":  "org/repo",
	}

	c, msg, gql := setupIntegrationConsumer(t, payload, attrs, "")

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("consumer.Run: %v", err)
	}

	msg.mu.Lock()
	acked := msg.acked
	nacked := msg.nacked
	msg.mu.Unlock()

	if !acked {
		t.Error("expected message to be acked on no match")
	}
	if nacked {
		t.Error("expected message not to be nacked")
	}

	requests := gql.getRequests()
	if len(requests) != 0 {
		t.Errorf("expected 0 GraphQL requests, got %d", len(requests))
	}
}

// TestIntegration_InvalidPayload_NackOnFailure verifies that invalid JSON
// payloads cause the consumer to nack the message when on_failure=nack.
func TestIntegration_InvalidPayload_NackOnFailure(t *testing.T) {
	c, msg, gql := setupIntegrationConsumer(t, []byte("not-valid-json"), map[string]string{
		"gh_event":    "issues",
		"action":      "opened",
		"gh_delivery": "delivery-4",
	}, "nack")

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("consumer.Run: %v", err)
	}

	msg.mu.Lock()
	acked := msg.acked
	nacked := msg.nacked
	msg.mu.Unlock()

	if acked {
		t.Error("expected message not to be acked on invalid payload with on_failure=nack")
	}
	if !nacked {
		t.Error("expected message to be nacked on invalid payload with on_failure=nack")
	}

	requests := gql.getRequests()
	if len(requests) != 0 {
		t.Errorf("expected 0 GraphQL requests for invalid payload, got %d", len(requests))
	}
}

// TestIntegration_InvalidPayload_AckOnFailure verifies that invalid payloads
// are acked when on_failure=ack.
func TestIntegration_InvalidPayload_AckOnFailure(t *testing.T) {
	c, msg, gql := setupIntegrationConsumer(t, []byte("{invalid json}"), map[string]string{
		"gh_event":    "issues",
		"action":      "opened",
		"gh_delivery": "delivery-5",
	}, "ack")

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("consumer.Run: %v", err)
	}

	msg.mu.Lock()
	acked := msg.acked
	nacked := msg.nacked
	msg.mu.Unlock()

	if !acked {
		t.Error("expected message to be acked on invalid payload with on_failure=ack")
	}
	if nacked {
		t.Error("expected message not to be nacked")
	}

	requests := gql.getRequests()
	if len(requests) != 0 {
		t.Errorf("expected 0 GraphQL requests for invalid payload, got %d", len(requests))
	}
}

// TestIntegration_GraphQLError_NackOnFailure verifies that GraphQL errors
// propagate through the consumer and result in nack when on_failure=nack.
func TestIntegration_GraphQLError_NackOnFailure(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-integration-token")

	cfg, err := config.Load("examples/config.yaml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	// GraphQL server that returns errors
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer srv.Close()

	cfg.GitHub.GraphQLEndpoint = srv.URL

	if err := config.Validate(cfg); err != nil {
		t.Fatalf("config.Validate: %v", err)
	}

	engine, err := rule.NewEngine(cfg.Rules, cfg.Constants, cfg.GitHub.GraphQLEndpoint, "test-integration-token")
	if err != nil {
		t.Fatalf("rule.NewEngine: %v", err)
	}

	payload := []byte(`{
		"repository": {"visibility": "public", "private": false, "full_name": "org/repo"},
		"pull_request": {"node_id": "PR_error_test"}
	}`)

	msg := &integrationMockMessage{
		id:   "integration-msg-error",
		data: payload,
		attributes: map[string]string{
			"gh_event":    "pull_request",
			"action":      "opened",
			"gh_delivery": "delivery-error",
			"repository":  "org/repo",
		},
	}

	h := health.NewStatus()
	h.SetAlive(true)

	c := consumer.New(
		cfg.PubSub.ProjectID,
		cfg.PubSub.SubscriptionID,
		"nack",
		engine,
		h,
		consumer.WithClientFactory(integrationMockFactory(msg)),
	)

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("consumer.Run: %v", err)
	}

	msg.mu.Lock()
	acked := msg.acked
	nacked := msg.nacked
	msg.mu.Unlock()

	if acked {
		t.Error("expected message not to be acked on GraphQL error with on_failure=nack")
	}
	if !nacked {
		t.Error("expected message to be nacked on GraphQL error with on_failure=nack")
	}
}

// TestIntegration_HealthStatusDuringConsumer verifies that the health status
// transitions correctly during consumer lifecycle.
func TestIntegration_HealthStatusDuringConsumer(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-integration-token")

	cfg, err := config.Load("examples/config.yaml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	gqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"addProjectV2ItemById": map[string]interface{}{
					"item": map[string]interface{}{"id": "PVTI_health_test"},
				},
			},
		})
	}))
	defer gqlSrv.Close()

	cfg.GitHub.GraphQLEndpoint = gqlSrv.URL

	engine, err := rule.NewEngine(cfg.Rules, cfg.Constants, cfg.GitHub.GraphQLEndpoint, "test-integration-token")
	if err != nil {
		t.Fatalf("rule.NewEngine: %v", err)
	}

	payload := []byte(`{"ref": "refs/heads/main", "repository": {"full_name": "org/repo"}}`)
	msg := &integrationMockMessage{
		id:   "integration-msg-health",
		data: payload,
		attributes: map[string]string{
			"gh_event":    "push",
			"action":      "completed",
			"gh_delivery": "delivery-health",
		},
	}

	h := health.NewStatus()
	h.SetAlive(true)

	// Verify not ready before Run
	if h.IsReady() {
		t.Error("expected not ready before Run")
	}

	// Create a subscription that checks health during receive
	var readyDuringReceive bool
	customSub := &healthCheckSubscription{
		msg:    msg,
		health: h,
		readyDuringReceive: &readyDuringReceive,
	}

	c := consumer.New(
		cfg.PubSub.ProjectID,
		cfg.PubSub.SubscriptionID,
		cfg.Behavior.OnFailure,
		engine,
		h,
		consumer.WithClientFactory(func(ctx context.Context, projectID string) (consumer.PubSubClient, error) {
			return &integrationMockClient{sub: customSub}, nil
		}),
	)

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("consumer.Run: %v", err)
	}

	if !readyDuringReceive {
		t.Error("expected health to be ready during message processing")
	}

	// After Run completes, ready should be false
	if h.IsReady() {
		t.Error("expected not ready after Run completes")
	}
}

// healthCheckSubscription captures the health ready state during Receive.
type healthCheckSubscription struct {
	msg                consumer.PubSubMessage
	health             *health.Status
	readyDuringReceive *bool
}

func (s *healthCheckSubscription) Receive(ctx context.Context, f func(ctx context.Context, msg consumer.PubSubMessage)) error {
	*s.readyDuringReceive = s.health.IsReady()
	f(ctx, s.msg)
	return nil
}

// TestIntegration_CoalesceFunction verifies that the coalesce function
// correctly picks the first non-nil value in the full pipeline.
// For an issue event, payload.pull_request.node_id is absent, so
// coalesce should fall back to payload.issue.node_id.
func TestIntegration_CoalesceFunction(t *testing.T) {
	payload := []byte(`{
		"repository": {"visibility": "public", "private": false, "full_name": "org/repo"},
		"issue": {"node_id": "I_coalesce_test_789"}
	}`)
	attrs := map[string]string{
		"gh_event":    "issues",
		"action":      "opened",
		"gh_delivery": "delivery-coalesce",
		"repository":  "org/repo",
	}

	c, msg, gql := setupIntegrationConsumer(t, payload, attrs, "")

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("consumer.Run: %v", err)
	}

	msg.mu.Lock()
	acked := msg.acked
	msg.mu.Unlock()

	if !acked {
		t.Error("expected message to be acked")
	}

	requests := gql.getRequests()
	if len(requests) != 2 {
		t.Fatalf("expected 2 GraphQL requests, got %d", len(requests))
	}

	// The coalesce function should have resolved to the issue node_id
	if requests[0].Variables["contentId"] != "I_coalesce_test_789" {
		t.Errorf("coalesce resolved contentId = %v, want I_coalesce_test_789", requests[0].Variables["contentId"])
	}
}
