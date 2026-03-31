package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/config"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/event"
)

func buildTestEvent(t *testing.T) *event.Event {
	t.Helper()
	payload := `{"repository":{"visibility":"public","private":false},"pull_request":{"node_id":"PR_123"}}`
	attrs := map[string]string{"gh_event": "pull_request", "action": "opened"}
	evt, err := event.BuildEvent([]byte(payload), attrs)
	if err != nil {
		t.Fatalf("BuildEvent failed: %v", err)
	}
	return evt
}

func TestNoRulesMatch(t *testing.T) {
	rules := []config.Rule{
		{
			Name:    "no-match",
			Enabled: true,
			When:    `meta.event_type = "push"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
	}

	eng, err := NewEngine(rules, nil, "", "")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	matched, name, err := eng.ProcessEvent(context.Background(), buildTestEvent(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched {
		t.Errorf("expected no match, got matched rule %q", name)
	}
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
}

func TestFirstMatchingRuleSelected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{}})
	}))
	defer srv.Close()

	rules := []config.Rule{
		{
			Name:    "rule-1",
			Enabled: true,
			When:    `meta.event_type = "pull_request"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
		{
			Name:    "rule-2",
			Enabled: true,
			When:    `meta.event_type = "pull_request"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
	}

	eng, err := NewEngine(rules, nil, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	matched, name, err := eng.ProcessEvent(context.Background(), buildTestEvent(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected a match")
	}
	if name != "rule-1" {
		t.Errorf("expected rule-1, got %q", name)
	}
}

func TestDisabledRuleSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{}})
	}))
	defer srv.Close()

	rules := []config.Rule{
		{
			Name:    "disabled-rule",
			Enabled: false,
			When:    `meta.event_type = "pull_request"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
		{
			Name:    "enabled-rule",
			Enabled: true,
			When:    `meta.event_type = "pull_request"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
	}

	eng, err := NewEngine(rules, nil, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	matched, name, err := eng.ProcessEvent(context.Background(), buildTestEvent(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected a match")
	}
	if name != "enabled-rule" {
		t.Errorf("expected enabled-rule, got %q", name)
	}
}

func TestInvalidWhenExpression(t *testing.T) {
	rules := []config.Rule{
		{
			Name:    "bad-rule",
			Enabled: true,
			When:    `meta.event_type ==== "pull_request"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
	}

	_, err := NewEngine(rules, nil, "", "")
	if err == nil {
		t.Fatal("expected error from invalid when expression")
	}
}

func TestStepExecutionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "server error"})
	}))
	defer srv.Close()

	rules := []config.Rule{
		{
			Name:    "fail-rule",
			Enabled: true,
			When:    `meta.event_type = "pull_request"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
	}

	eng, err := NewEngine(rules, nil, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	matched, name, err := eng.ProcessEvent(context.Background(), buildTestEvent(t))
	if err == nil {
		t.Fatal("expected error from failed step execution")
	}
	if !matched {
		t.Error("expected matched to be true even on step failure")
	}
	if name != "fail-rule" {
		t.Errorf("expected fail-rule, got %q", name)
	}
}

func TestTwoStepExecutionPassesOutputs(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		w.Header().Set("Content-Type", "application/json")

		if callCount == 0 {
			// First step: return a label ID
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"repository": map[string]interface{}{
						"label": map[string]interface{}{
							"id": "LABEL_456",
						},
					},
				},
			})
		} else {
			// Second step: verify the variable from step 1 was passed
			vars, _ := reqBody["variables"].(map[string]interface{})
			labelID, _ := vars["labelId"].(string)
			if labelID != "LABEL_456" {
				t.Errorf("expected labelId=LABEL_456, got %q", labelID)
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{map[string]interface{}{"message": "bad labelId"}}})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"addLabelsToLabelable": map[string]interface{}{
						"clientMutationId": "done",
					},
				},
			})
		}
		callCount++
	}))
	defer srv.Close()

	rules := []config.Rule{
		{
			Name:    "two-step-rule",
			Enabled: true,
			When:    `meta.event_type = "pull_request"`,
			Then: []config.Step{
				{
					Name: "lookup",
					Type: "github_graphql",
					Config: config.StepConfig{
						Document: `query { repository(owner:"o", name:"r") { label(name:"bug") { id } } }`,
						Outputs: map[string]string{
							"labelId": "data.repository.label.id",
						},
					},
				},
				{
					Name: "apply",
					Type: "github_graphql",
					Config: config.StepConfig{
						Document: `mutation AddLabel($labelId: ID!, $prId: ID!) { addLabelsToLabelable(input:{labelableId:$prId, labelIds:[$labelId]}) { clientMutationId } }`,
						Variables: map[string]string{
							"labelId": "${steps.lookup.outputs.labelId}",
							"prId":    "${payload.pull_request.node_id}",
						},
						Outputs: map[string]string{
							"mutationId": "data.addLabelsToLabelable.clientMutationId",
						},
					},
				},
			},
		},
	}

	eng, err := NewEngine(rules, nil, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	matched, name, err := eng.ProcessEvent(context.Background(), buildTestEvent(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected a match")
	}
	if name != "two-step-rule" {
		t.Errorf("expected two-step-rule, got %q", name)
	}
	if callCount != 2 {
		t.Errorf("expected 2 GraphQL calls, got %d", callCount)
	}
}

func TestSecondRuleMatchesWhenFirstDoesNot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{}})
	}))
	defer srv.Close()

	rules := []config.Rule{
		{
			Name:    "push-rule",
			Enabled: true,
			When:    `meta.event_type = "push"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
		{
			Name:    "pr-rule",
			Enabled: true,
			When:    `meta.event_type = "pull_request"`,
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
	}

	eng, err := NewEngine(rules, nil, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	matched, name, err := eng.ProcessEvent(context.Background(), buildTestEvent(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected a match")
	}
	if name != "pr-rule" {
		t.Errorf("expected pr-rule, got %q", name)
	}
}

func TestStepExecutionFailureStopsRemaining(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// First step returns a GraphQL error
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []interface{}{
				map[string]interface{}{"message": "not found"},
			},
		})
	}))
	defer srv.Close()

	rules := []config.Rule{
		{
			Name:    "multi-step",
			Enabled: true,
			When:    `meta.event_type = "pull_request"`,
			Then: []config.Step{
				{Name: "step1", Type: "github_graphql", Config: config.StepConfig{Document: "query {}"}},
				{Name: "step2", Type: "github_graphql", Config: config.StepConfig{Document: "mutation {}"}},
			},
		},
	}

	eng, err := NewEngine(rules, nil, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	_, _, err = eng.ProcessEvent(context.Background(), buildTestEvent(t))
	if err == nil {
		t.Fatal("expected error from first step failure")
	}
	if callCount != 1 {
		t.Errorf("expected only 1 call (second step should not execute), got %d", callCount)
	}
}

// errorExpr is a test helper that shows the error message format.
func TestNewEngineErrorMessage(t *testing.T) {
	rules := []config.Rule{
		{
			Name:    "broken",
			Enabled: true,
			When:    "!!!",
			Then: []config.Step{
				{Name: "s1", Type: "github_graphql", Config: config.StepConfig{Document: "q"}},
			},
		},
	}

	_, err := NewEngine(rules, nil, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	expected := fmt.Sprintf("compiling rule %q", "broken")
	if got := err.Error(); len(got) < len(expected) || got[:len(expected)] != expected {
		t.Errorf("error should start with %q, got %q", expected, got)
	}
}
