package step

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/config"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/resolve"
)

func TestExecute_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Authorization 'Bearer test-token', got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", got)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if body["query"] != "mutation AddItem($input: ID!) { addProjectV2ItemById(input: {projectId: $input}) { item { id } } }" {
			t.Errorf("unexpected query: %v", body["query"])
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"addProjectV2ItemById": map[string]interface{}{
					"item": map[string]interface{}{
						"id": "ITEM_123",
					},
				},
			},
		})
	}))
	defer server.Close()

	executor := &GraphQLExecutor{
		Endpoint: server.URL,
		Token:    "test-token",
		Client:   server.Client(),
	}

	step := &config.Step{
		Name: "add-item",
		Type: "github_graphql",
		Config: config.StepConfig{
			Document:  "mutation AddItem($input: ID!) { addProjectV2ItemById(input: {projectId: $input}) { item { id } } }",
			Variables: map[string]string{"input": "proj-abc"},
			Outputs:   map[string]string{"itemId": "data.addProjectV2ItemById.item.id"},
		},
	}

	resolveCtx := &resolve.ResolveContext{}

	outputs, err := executor.Execute(context.Background(), step, resolveCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if outputs["itemId"] != "ITEM_123" {
		t.Errorf("expected itemId=ITEM_123, got %v", outputs["itemId"])
	}
}

func TestExecute_GraphQLErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":   nil,
			"errors": []interface{}{map[string]interface{}{"message": "Could not resolve to a node"}},
		})
	}))
	defer server.Close()

	executor := &GraphQLExecutor{
		Endpoint: server.URL,
		Token:    "test-token",
		Client:   server.Client(),
	}

	step := &config.Step{
		Name: "bad-step",
		Type: "github_graphql",
		Config: config.StepConfig{
			Document:  "query { viewer { login } }",
			Variables: map[string]string{},
			Outputs:   map[string]string{},
		},
	}

	_, err := executor.Execute(context.Background(), step, &resolve.ResolveContext{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "GraphQL error: Could not resolve to a node" {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestExecute_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Bad credentials",
		})
	}))
	defer server.Close()

	executor := &GraphQLExecutor{
		Endpoint: server.URL,
		Token:    "bad-token",
		Client:   server.Client(),
	}

	step := &config.Step{
		Name: "auth-fail",
		Type: "github_graphql",
		Config: config.StepConfig{
			Document:  "query { viewer { login } }",
			Variables: map[string]string{},
			Outputs:   map[string]string{},
		},
	}

	_, err := executor.Execute(context.Background(), step, &resolve.ResolveContext{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	expected := "GraphQL request failed with status 401"
	if got := err.Error(); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestExecute_VariableResolutionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called when variable resolution fails")
	}))
	defer server.Close()

	executor := &GraphQLExecutor{
		Endpoint: server.URL,
		Token:    "test-token",
		Client:   server.Client(),
	}

	step := &config.Step{
		Name: "bad-vars",
		Type: "github_graphql",
		Config: config.StepConfig{
			Document: "mutation { test }",
			Variables: map[string]string{
				"projectId": "${payload.nonexistent.field}",
			},
			Outputs: map[string]string{},
		},
	}

	// ResolveContext with no event — payload references will fail
	resolveCtx := &resolve.ResolveContext{}

	_, err := executor.Execute(context.Background(), step, resolveCtx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "resolving variables") {
		t.Errorf("expected error about resolving variables, got: %s", got)
	}
}

func TestExecute_NestedOutputExtraction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"issue": map[string]interface{}{
						"id":     "ISSUE_456",
						"number": float64(42),
						"title":  "Fix the bug",
						"author": map[string]interface{}{
							"login": "octocat",
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	executor := &GraphQLExecutor{
		Endpoint: server.URL,
		Token:    "test-token",
		Client:   server.Client(),
	}

	step := &config.Step{
		Name: "get-issue",
		Type: "github_graphql",
		Config: config.StepConfig{
			Document:  "query { repository(owner:\"o\", name:\"r\") { issue(number:42) { id number title author { login } } } }",
			Variables: map[string]string{},
			Outputs: map[string]string{
				"issueId":     "data.repository.issue.id",
				"issueNumber": "data.repository.issue.number",
				"authorLogin": "data.repository.issue.author.login",
			},
		},
	}

	outputs, err := executor.Execute(context.Background(), step, &resolve.ResolveContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if outputs["issueId"] != "ISSUE_456" {
		t.Errorf("expected issueId=ISSUE_456, got %v", outputs["issueId"])
	}
	if outputs["issueNumber"] != float64(42) {
		t.Errorf("expected issueNumber=42, got %v", outputs["issueNumber"])
	}
	if outputs["authorLogin"] != "octocat" {
		t.Errorf("expected authorLogin=octocat, got %v", outputs["authorLogin"])
	}
}

func TestGetExecutor_UnknownType(t *testing.T) {
	_, err := GetExecutor("unknown_type", "http://example.com", "token")
	if err == nil {
		t.Fatal("expected error for unknown step type, got nil")
	}
	expected := `unknown step type: "unknown_type"`
	if got := err.Error(); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestExecute_TwoStepChain(t *testing.T) {
	// Simulate: step 1 adds an item, step 2 updates a field using step 1's output
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		callCount++
		switch callCount {
		case 1:
			// Step 1: addProjectV2ItemById
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"addProjectV2ItemById": map[string]interface{}{
						"item": map[string]interface{}{
							"id": "PVTI_item123",
						},
					},
				},
			})
		case 2:
			// Step 2: updateProjectV2ItemFieldValue — verify it received step 1's output
			vars := body["variables"].(map[string]interface{})
			if vars["itemId"] != "PVTI_item123" {
				t.Errorf("step 2 expected itemId=PVTI_item123, got %v", vars["itemId"])
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"updateProjectV2ItemFieldValue": map[string]interface{}{
						"projectV2Item": map[string]interface{}{
							"id": "PVTI_item123",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected call %d", callCount)
		}
	}))
	defer server.Close()

	executor := &GraphQLExecutor{
		Endpoint: server.URL,
		Token:    "test-token",
		Client:   server.Client(),
	}

	resolveCtx := &resolve.ResolveContext{
		Constants: map[string]interface{}{
			"project_id":    "PVT_proj456",
			"status_field":  "FIELD_status",
			"status_option": "OPT_done",
		},
		Steps: make(map[string]map[string]interface{}),
	}

	// Step 1: Add item to project
	step1 := &config.Step{
		Name: "add-to-project",
		Type: "github_graphql",
		Config: config.StepConfig{
			Document: "mutation AddItem($projectId: ID!, $contentId: ID!) { addProjectV2ItemById(input: {projectId: $projectId, contentId: $contentId}) { item { id } } }",
			Variables: map[string]string{
				"projectId": "${constants.project_id}",
				"contentId": "CONTENT_abc",
			},
			Outputs: map[string]string{
				"itemId": "data.addProjectV2ItemById.item.id",
			},
		},
	}

	outputs1, err := executor.Execute(context.Background(), step1, resolveCtx)
	if err != nil {
		t.Fatalf("step 1 failed: %v", err)
	}
	if outputs1["itemId"] != "PVTI_item123" {
		t.Fatalf("step 1: expected itemId=PVTI_item123, got %v", outputs1["itemId"])
	}

	// Register step 1 outputs so step 2 can reference them
	resolveCtx.Steps["add-to-project"] = outputs1

	// Step 2: Update field value using step 1's output
	step2 := &config.Step{
		Name: "set-status",
		Type: "github_graphql",
		Config: config.StepConfig{
			Document: "mutation UpdateField($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String!) { updateProjectV2ItemFieldValue(input: {projectId: $projectId, itemId: $itemId, fieldId: $fieldId, value: {singleSelectOptionId: $optionId}}) { projectV2Item { id } } }",
			Variables: map[string]string{
				"projectId": "${constants.project_id}",
				"itemId":    "${steps.add-to-project.outputs.itemId}",
				"fieldId":   "${constants.status_field}",
				"optionId":  "${constants.status_option}",
			},
			Outputs: map[string]string{
				"updatedItemId": "data.updateProjectV2ItemFieldValue.projectV2Item.id",
			},
		},
	}

	outputs2, err := executor.Execute(context.Background(), step2, resolveCtx)
	if err != nil {
		t.Fatalf("step 2 failed: %v", err)
	}
	if outputs2["updatedItemId"] != "PVTI_item123" {
		t.Errorf("step 2: expected updatedItemId=PVTI_item123, got %v", outputs2["updatedItemId"])
	}
}

func TestExtractField(t *testing.T) {
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"nested": map[string]interface{}{
				"value": "found",
			},
		},
	}

	val, ok := extractField(data, "data.nested.value")
	if !ok {
		t.Fatal("expected field to be found")
	}
	if val != "found" {
		t.Errorf("expected 'found', got %v", val)
	}

	_, ok = extractField(data, "data.nonexistent.path")
	if ok {
		t.Error("expected field to not be found")
	}
}
