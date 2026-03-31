package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/config"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/health"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/rule"
)

// --- Mock implementations ---

type mockMessage struct {
	mu         sync.Mutex
	id         string
	data       []byte
	attributes map[string]string
	acked      bool
	nacked     bool
}

func (m *mockMessage) ID() string                    { return m.id }
func (m *mockMessage) Data() []byte                  { return m.data }
func (m *mockMessage) Attributes() map[string]string { return m.attributes }

func (m *mockMessage) Ack() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acked = true
}

func (m *mockMessage) Nack() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nacked = true
}

type mockSubscription struct {
	msg PubSubMessage
}

func (ms *mockSubscription) Receive(ctx context.Context, f func(ctx context.Context, msg PubSubMessage)) error {
	f(ctx, ms.msg)
	return nil
}

type mockClient struct {
	sub PubSubSubscription
}

func (mc *mockClient) Subscription(id string) PubSubSubscription {
	return mc.sub
}

func (mc *mockClient) Close() error {
	return nil
}

// --- Helpers ---

func validPayload(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{
		"action": "opened",
		"repository": map[string]interface{}{
			"full_name": "owner/repo",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newMockMessage(t *testing.T, data []byte, attrs map[string]string) *mockMessage {
	t.Helper()
	return &mockMessage{
		id:         "msg-123",
		data:       data,
		attributes: attrs,
	}
}

func mockFactory(msg PubSubMessage) func(ctx context.Context, projectID string) (PubSubClient, error) {
	return func(ctx context.Context, projectID string) (PubSubClient, error) {
		return &mockClient{
			sub: &mockSubscription{msg: msg},
		}, nil
	}
}

func newGraphQLServer(t *testing.T, statusCode int, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		fmt.Fprint(w, responseBody)
	}))
}

func buildEngine(t *testing.T, whenExpr string, endpoint string) *rule.Engine {
	t.Helper()
	rules := []config.Rule{
		{
			Name:    "test-rule",
			Enabled: true,
			When:    whenExpr,
			Then: []config.Step{
				{
					Name: "step1",
					Type: "github_graphql",
					Config: config.StepConfig{
						Document: `mutation { addLabel(input: {}) { clientMutationId } }`,
					},
				},
			},
		},
	}
	eng, err := rule.NewEngine(rules, nil, endpoint, "fake-token")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	return eng
}

// --- Tests ---

func TestSuccessfulMatchingRule(t *testing.T) {
	srv := newGraphQLServer(t, http.StatusOK, `{"data":{"addLabel":{"clientMutationId":"abc"}}}`)
	defer srv.Close()

	eng := buildEngine(t, `payload.action = "opened"`, srv.URL)
	msg := newMockMessage(t, validPayload(t), map[string]string{
		"gh_event":    "issues",
		"action":      "opened",
		"gh_delivery": "del-1",
		"repository":  "owner/repo",
	})
	h := health.NewStatus()

	c := New("proj", "sub", "ack", eng, h,
		WithClientFactory(mockFactory(msg)),
	)

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !msg.acked {
		t.Error("expected message to be acked")
	}
	if msg.nacked {
		t.Error("expected message not to be nacked")
	}
}

func TestNoMatchingRule(t *testing.T) {
	// Use an expression that won't match
	eng := buildEngine(t, `payload.action = "closed"`, "http://unused")
	msg := newMockMessage(t, validPayload(t), map[string]string{
		"gh_event": "issues",
		"action":   "opened",
	})
	h := health.NewStatus()

	c := New("proj", "sub", "ack", eng, h,
		WithClientFactory(mockFactory(msg)),
	)

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !msg.acked {
		t.Error("expected message to be acked on no match")
	}
	if msg.nacked {
		t.Error("expected message not to be nacked")
	}
}

func TestEventBuildFailure_OnFailureAck(t *testing.T) {
	eng := buildEngine(t, `payload.action = "opened"`, "http://unused")
	// Invalid JSON to cause BuildEvent to fail
	msg := newMockMessage(t, []byte("not-json"), map[string]string{})
	h := health.NewStatus()

	c := New("proj", "sub", "ack", eng, h,
		WithClientFactory(mockFactory(msg)),
	)

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !msg.acked {
		t.Error("expected message to be acked on event build failure with on_failure=ack")
	}
	if msg.nacked {
		t.Error("expected message not to be nacked")
	}
}

func TestEventBuildFailure_OnFailureNack(t *testing.T) {
	eng := buildEngine(t, `payload.action = "opened"`, "http://unused")
	msg := newMockMessage(t, []byte("not-json"), map[string]string{})
	h := health.NewStatus()

	c := New("proj", "sub", "nack", eng, h,
		WithClientFactory(mockFactory(msg)),
	)

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if msg.acked {
		t.Error("expected message not to be acked")
	}
	if !msg.nacked {
		t.Error("expected message to be nacked on event build failure with on_failure=nack")
	}
}

func TestStepExecutionFailure_OnFailureNack(t *testing.T) {
	// Server returns error to cause step execution to fail
	srv := newGraphQLServer(t, http.StatusInternalServerError, `server error`)
	defer srv.Close()

	eng := buildEngine(t, `payload.action = "opened"`, srv.URL)
	msg := newMockMessage(t, validPayload(t), map[string]string{
		"gh_event": "issues",
		"action":   "opened",
	})
	h := health.NewStatus()

	c := New("proj", "sub", "nack", eng, h,
		WithClientFactory(mockFactory(msg)),
	)

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if msg.acked {
		t.Error("expected message not to be acked on step failure with on_failure=nack")
	}
	if !msg.nacked {
		t.Error("expected message to be nacked on step failure with on_failure=nack")
	}
}

func TestStepExecutionFailure_OnFailureAck(t *testing.T) {
	srv := newGraphQLServer(t, http.StatusInternalServerError, `server error`)
	defer srv.Close()

	eng := buildEngine(t, `payload.action = "opened"`, srv.URL)
	msg := newMockMessage(t, validPayload(t), map[string]string{
		"gh_event": "issues",
		"action":   "opened",
	})
	h := health.NewStatus()

	c := New("proj", "sub", "ack", eng, h,
		WithClientFactory(mockFactory(msg)),
	)

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !msg.acked {
		t.Error("expected message to be acked on step failure with on_failure=ack")
	}
	if msg.nacked {
		t.Error("expected message not to be nacked")
	}
}
