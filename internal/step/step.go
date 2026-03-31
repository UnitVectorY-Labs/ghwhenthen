package step

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/config"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/resolve"
)

// Executor executes a step and returns outputs.
type Executor interface {
	Execute(ctx context.Context, step *config.Step, resolveCtx *resolve.ResolveContext) (map[string]interface{}, error)
}

// GraphQLExecutor executes github_graphql steps.
type GraphQLExecutor struct {
	Endpoint string
	Token    string
	Client   *http.Client // allow injection for testing
}

// NewGraphQLExecutor creates a new GraphQLExecutor with default http.Client.
func NewGraphQLExecutor(endpoint, token string) *GraphQLExecutor {
	return &GraphQLExecutor{
		Endpoint: endpoint,
		Token:    token,
		Client:   http.DefaultClient,
	}
}

// Execute runs a github_graphql step: resolves variables, sends the GraphQL
// request, checks for errors, and extracts outputs.
func (g *GraphQLExecutor) Execute(ctx context.Context, step *config.Step, resolveCtx *resolve.ResolveContext) (map[string]interface{}, error) {
	// 1. Resolve variables
	resolved, err := resolve.ResolveMap(step.Config.Variables, resolveCtx)
	if err != nil {
		return nil, fmt.Errorf("resolving variables: %w", err)
	}

	// 2. Build GraphQL request body
	body := map[string]interface{}{
		"query":     step.Config.Document,
		"variables": resolved,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}

	// 3. Send HTTP POST
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// 4. Parse JSON response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
	}

	// 5. Check for GraphQL errors
	if errs, ok := result["errors"]; ok {
		if errSlice, ok := errs.([]interface{}); ok && len(errSlice) > 0 {
			if first, ok := errSlice[0].(map[string]interface{}); ok {
				if msg, ok := first["message"].(string); ok {
					return nil, fmt.Errorf("GraphQL error: %s", msg)
				}
			}
			return nil, fmt.Errorf("GraphQL error: %v", errSlice[0])
		}
	}

	// 6. Extract outputs
	outputs := make(map[string]interface{})
	for name, path := range step.Config.Outputs {
		val, ok := extractField(result, path)
		if !ok {
			return nil, fmt.Errorf("output %q: field %q not found in response", name, path)
		}
		outputs[name] = val
	}

	return outputs, nil
}

// extractField traverses a nested map using a dot-separated path.
func extractField(data interface{}, path string) (interface{}, bool) {
	keys := strings.Split(path, ".")
	current := data
	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		val, ok := m[key]
		if !ok {
			return nil, false
		}
		current = val
	}
	return current, true
}

// GetExecutor returns the right executor for a step type.
func GetExecutor(stepType string, endpoint string, token string) (Executor, error) {
	switch stepType {
	case "github_graphql":
		return NewGraphQLExecutor(endpoint, token), nil
	default:
		return nil, fmt.Errorf("unknown step type: %q", stepType)
	}
}
