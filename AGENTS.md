# AGENTS.md

## Project Overview

**ghwhenthen** is a Go service that bridges GitHub webhooks and GitHub's GraphQL API through Google Cloud Pub/Sub. It consumes webhook events, evaluates declarative YAML "when/then" rules, and executes ordered GraphQL mutations.

## Setup

- **Language:** Go 1.26+
- **Dependencies:** `go mod download`
- **Build:** `go build ./...`
- **Test:** `go test ./...`
- **Lint:** Standard `go vet ./...`

## Architecture

```
Pub/Sub Message → consumer.Consumer → event.BuildEvent → rule.Engine.ProcessEvent
  → expr.Expression.Evaluate → step.GraphQLExecutor.Execute → resolve.ResolveMap
```

### Packages

| Package | Purpose |
|---------|---------|
| `internal/config` | YAML config loading and validation |
| `internal/consumer` | Pub/Sub message consumer with injectable client factory |
| `internal/event` | Event model with gzip/zstd decompression |
| `internal/expr` | Expression parser (`=`, `!=`, `AND`, `OR`, `has()`) |
| `internal/health` | Kubernetes `/livez` and `/readyz` endpoints |
| `internal/resolve` | `${...}` variable interpolation and `coalesce()` |
| `internal/rule` | Rule engine: compiles when-expressions, executes then-steps |
| `internal/step` | GraphQL executor with output extraction |

### Key Interfaces

The codebase uses **interface-based dependency injection** for testability. No external mocking library is used.

- `consumer.PubSubClient` — wraps the Pub/Sub client
- `consumer.PubSubSubscription` — wraps the Pub/Sub subscription
- `consumer.PubSubMessage` — wraps individual messages (ID, Data, Attributes, Ack, Nack)
- `step.Executor` — executes a step and returns outputs

## Code Conventions

- No external test libraries; use standard `testing` package
- Use `t.Helper()` in test helper functions
- Use `t.Setenv()` for environment variable isolation
- Use `t.Cleanup()` for resource cleanup
- Use `httptest.NewServer` for HTTP service mocking
- Use `sync.Mutex` for thread-safe test state
- Structured logging via `log/slog` (JSON handler)
- Error wrapping with `fmt.Errorf("context: %w", err)`

## Testing

### Running Tests

```bash
go test ./...                    # All tests
go test -v -run TestIntegration  # Integration tests only
go test -v -run TestE2E          # E2E tests only
go test -v ./internal/consumer/  # Consumer package only
```

### Test Structure

| File | Scope | What It Tests |
|------|-------|---------------|
| `integration_test.go` | Full stack | Pub/Sub mock → Consumer → Config → Rules → GraphQL mock |
| `e2e_test.go` | Rule engine + GraphQL | Event → Rule matching → Multi-step GraphQL with output chaining |
| `internal/consumer/consumer_test.go` | Consumer | Message handling, ack/nack behavior, error paths |
| `internal/step/step_test.go` | GraphQL executor | Request/response handling, output extraction, step chaining |
| `internal/*/..._test.go` | Unit tests | Config parsing, expressions, events, health, resolution |

### Mock Testing Approach

All external dependencies are mocked using Go interfaces and `httptest`:

#### Mocking Google Cloud Pub/Sub

Implement the three consumer interfaces. Use `consumer.WithClientFactory()` to inject:

```go
// Message mock — tracks ack/nack state
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
func (m *mockMessage) Ack()  { m.mu.Lock(); defer m.mu.Unlock(); m.acked = true }
func (m *mockMessage) Nack() { m.mu.Lock(); defer m.mu.Unlock(); m.nacked = true }

// Subscription mock — delivers a single message
type mockSubscription struct{ msg consumer.PubSubMessage }
func (ms *mockSubscription) Receive(ctx context.Context, f func(context.Context, consumer.PubSubMessage)) error {
    f(ctx, ms.msg)
    return nil
}

// Client mock
type mockClient struct{ sub consumer.PubSubSubscription }
func (mc *mockClient) Subscription(id string) consumer.PubSubSubscription { return mc.sub }
func (mc *mockClient) Close() error { return nil }

// Inject via WithClientFactory option:
c := consumer.New(projectID, subID, onFailure, engine, health,
    consumer.WithClientFactory(func(ctx context.Context, pid string) (consumer.PubSubClient, error) {
        return &mockClient{sub: &mockSubscription{msg: myMsg}}, nil
    }),
)
```

#### Mocking GitHub GraphQL API

Use `httptest.NewServer` to create a mock GraphQL endpoint. Detect mutation type from request variables:

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Query     string                 `json:"query"`
        Variables map[string]interface{} `json:"variables"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    w.Header().Set("Content-Type", "application/json")

    // Return canned response based on mutation type
    if _, ok := req.Variables["contentId"]; ok {
        json.NewEncoder(w).Encode(map[string]interface{}{
            "data": map[string]interface{}{
                "addProjectV2ItemById": map[string]interface{}{
                    "item": map[string]interface{}{"id": "PVTI_mock_id"},
                },
            },
        })
        return
    }
    json.NewEncoder(w).Encode(map[string]interface{}{
        "data": map[string]interface{}{
            "updateProjectV2ItemFieldValue": map[string]interface{}{
                "projectV2Item": map[string]interface{}{"id": "PVTI_mock_id"},
            },
        },
    })
}))
defer srv.Close()
```

Override the config endpoint: `cfg.GitHub.GraphQLEndpoint = srv.URL`

#### Full Integration Test Pattern

Combine both mocks to test the complete flow:

1. Load `examples/config.yaml` and override `GraphQLEndpoint`
2. Set `GITHUB_TOKEN` via `t.Setenv()`
3. Create rule engine from config
4. Create mock Pub/Sub message with webhook payload + attributes
5. Create consumer with mock client factory
6. Call `consumer.Run()` and assert:
   - Message ack/nack state
   - Number and content of GraphQL requests
   - Output chaining between steps (step 1 output used in step 2)

See `integration_test.go` for complete examples.

### Test Data

Pub/Sub message attributes for GitHub webhook events:

```go
// Pull request opened (public repo)
attrs := map[string]string{
    "gh_event":    "pull_request",
    "action":      "opened",
    "gh_delivery": "delivery-id",
    "repository":  "org/repo",
}

// Issue opened (private repo)
attrs := map[string]string{
    "gh_event":    "issues",
    "action":      "opened",
    "gh_delivery": "delivery-id",
    "repository":  "org/repo",
}
```

Minimal webhook payloads:

```json
// Public PR
{"repository": {"visibility": "public", "private": false, "full_name": "org/repo"},
 "pull_request": {"node_id": "PR_kwDO123"}}

// Private issue
{"repository": {"visibility": "private", "private": true, "full_name": "org/repo"},
 "issue": {"node_id": "I_kwDO456"}}
```

## Configuration

Config is loaded from YAML (`examples/config.yaml` for reference). Key sections:
- `pubsub.project_id` / `pubsub.subscription_id` — GCP Pub/Sub settings
- `github.token_env_var` / `github.graphql_endpoint` — GitHub API settings
- `behavior.on_failure` — `"ack"` or `"nack"` on processing errors
- `constants` — Nested key-value pairs for variable resolution
- `rules[].when` — Expression evaluated against event attributes and payload
- `rules[].then[]` — Ordered steps with GraphQL documents, variables, and output extraction

## Boundaries

- Never commit real GitHub tokens or GCP credentials
- Do not modify `.github/workflows/` without explicit request
- Do not add external testing libraries; use the standard library
- Do not modify `go.mod` unless adding a new application dependency
- The `examples/config.yaml` file serves as test fixture; changes affect integration tests
