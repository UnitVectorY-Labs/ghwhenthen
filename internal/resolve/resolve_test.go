package resolve

import (
	"testing"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/event"
)

func makeTestContext() *ResolveContext {
	return &ResolveContext{
		Event: &event.Event{
			Attributes: map[string]string{
				"gh_event": "pull_request",
			},
			Payload: map[string]interface{}{
				"repository": map[string]interface{}{
					"visibility": "private",
				},
				"pull_request": map[string]interface{}{
					"node_id": "PR_abc123",
				},
			},
			Meta: map[string]string{
				"event_type": "pull_request.opened",
			},
		},
		Constants: map[string]interface{}{
			"projects": map[string]interface{}{
				"public": map[string]interface{}{
					"project_id": "PVT_123",
				},
			},
			"key":     "simple_value",
			"enabled": true,
		},
		Steps: map[string]map[string]interface{}{
			"add_to_project": {
				"itemId": "PVTI_456",
			},
		},
	}
}

func TestResolveConstantNested(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("${constants.projects.public.project_id}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "PVT_123" {
		t.Fatalf("expected PVT_123, got %v", val)
	}
}

func TestResolvePayload(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("${payload.repository.visibility}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "private" {
		t.Fatalf("expected private, got %v", val)
	}
}

func TestResolveAttributes(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("${attributes.gh_event}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "pull_request" {
		t.Fatalf("expected pull_request, got %v", val)
	}
}

func TestResolveMeta(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("${meta.event_type}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "pull_request.opened" {
		t.Fatalf("expected pull_request.opened, got %v", val)
	}
}

func TestResolveStepsOutput(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("${steps.add_to_project.outputs.itemId}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "PVTI_456" {
		t.Fatalf("expected PVTI_456, got %v", val)
	}
}

func TestResolveCoalesceFirstPresent(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("${coalesce(payload.pull_request.node_id, payload.issue.node_id)}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "PR_abc123" {
		t.Fatalf("expected PR_abc123, got %v", val)
	}
}

func TestResolveCoalesceSecondPresent(t *testing.T) {
	ctx := makeTestContext()
	// pull_request.title doesn't exist, so it should fall through to node_id.
	val, err := Resolve("${coalesce(payload.pull_request.title, payload.pull_request.node_id)}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "PR_abc123" {
		t.Fatalf("expected PR_abc123, got %v", val)
	}
}

func TestResolveCoalesceAllMissing(t *testing.T) {
	ctx := makeTestContext()
	_, err := Resolve("${coalesce(payload.nonexistent1, payload.nonexistent2)}", ctx)
	if err == nil {
		t.Fatal("expected error for coalesce with all missing values")
	}
}

func TestResolvePlainString(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("just a plain string", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "just a plain string" {
		t.Fatalf("expected plain string, got %v", val)
	}
}

func TestResolveMixedString(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("prefix-${constants.key}-suffix", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	str, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	if str != "prefix-simple_value-suffix" {
		t.Fatalf("expected prefix-simple_value-suffix, got %s", str)
	}
}

func TestResolveMissingReference(t *testing.T) {
	ctx := makeTestContext()
	_, err := Resolve("${payload.nonexistent.field}", ctx)
	if err == nil {
		t.Fatal("expected error for missing reference")
	}
}

func TestResolveMap(t *testing.T) {
	ctx := makeTestContext()
	m := map[string]string{
		"projectId": "${constants.projects.public.project_id}",
		"event":     "${attributes.gh_event}",
		"literal":   "no_reference",
	}
	result, err := ResolveMap(m, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["projectId"] != "PVT_123" {
		t.Fatalf("expected PVT_123, got %v", result["projectId"])
	}
	if result["event"] != "pull_request" {
		t.Fatalf("expected pull_request, got %v", result["event"])
	}
	if result["literal"] != "no_reference" {
		t.Fatalf("expected no_reference, got %v", result["literal"])
	}
}

func TestResolveWholeStringNonString(t *testing.T) {
	ctx := makeTestContext()
	val, err := Resolve("${constants.enabled}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, ok := val.(bool)
	if !ok {
		t.Fatalf("expected bool, got %T", val)
	}
	if b != true {
		t.Fatalf("expected true, got %v", b)
	}
}

func TestResolveStringConvertsBool(t *testing.T) {
	ctx := makeTestContext()
	val, err := ResolveString("${constants.enabled}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "true" {
		t.Fatalf("expected 'true', got %s", val)
	}
}
