package expr

import (
	"testing"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/event"
)

func makeEvent(attrs map[string]string, payload map[string]interface{}) *event.Event {
	return &event.Event{
		Attributes: attrs,
		Payload:    payload,
		Meta:       map[string]string{},
	}
}

func TestSimpleEquality(t *testing.T) {
	expr, err := Parse(`attributes.gh_event = "pull_request"`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(map[string]string{"gh_event": "pull_request"}, nil)
	if !expr.Evaluate(e) {
		t.Error("expected true")
	}
	e2 := makeEvent(map[string]string{"gh_event": "issues"}, nil)
	if expr.Evaluate(e2) {
		t.Error("expected false")
	}
}

func TestSimpleInequality(t *testing.T) {
	expr, err := Parse(`attributes.action != "closed"`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(map[string]string{"action": "opened"}, nil)
	if !expr.Evaluate(e) {
		t.Error("expected true")
	}
	e2 := makeEvent(map[string]string{"action": "closed"}, nil)
	if expr.Evaluate(e2) {
		t.Error("expected false")
	}
}

func TestAnd(t *testing.T) {
	expr, err := Parse(`attributes.gh_event = "pull_request" AND attributes.action = "opened"`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(map[string]string{"gh_event": "pull_request", "action": "opened"}, nil)
	if !expr.Evaluate(e) {
		t.Error("expected true")
	}
	e2 := makeEvent(map[string]string{"gh_event": "pull_request", "action": "closed"}, nil)
	if expr.Evaluate(e2) {
		t.Error("expected false")
	}
}

func TestOr(t *testing.T) {
	expr, err := Parse(`attributes.gh_event = "pull_request" OR attributes.gh_event = "issues"`)
	if err != nil {
		t.Fatal(err)
	}
	e1 := makeEvent(map[string]string{"gh_event": "pull_request"}, nil)
	if !expr.Evaluate(e1) {
		t.Error("expected true for pull_request")
	}
	e2 := makeEvent(map[string]string{"gh_event": "issues"}, nil)
	if !expr.Evaluate(e2) {
		t.Error("expected true for issues")
	}
	e3 := makeEvent(map[string]string{"gh_event": "push"}, nil)
	if expr.Evaluate(e3) {
		t.Error("expected false for push")
	}
}

func TestParentheses(t *testing.T) {
	expr, err := Parse(`(attributes.gh_event = "pull_request" OR attributes.gh_event = "issues") AND attributes.action = "opened"`)
	if err != nil {
		t.Fatal(err)
	}
	e1 := makeEvent(map[string]string{"gh_event": "pull_request", "action": "opened"}, nil)
	if !expr.Evaluate(e1) {
		t.Error("expected true")
	}
	e2 := makeEvent(map[string]string{"gh_event": "issues", "action": "opened"}, nil)
	if !expr.Evaluate(e2) {
		t.Error("expected true")
	}
	e3 := makeEvent(map[string]string{"gh_event": "pull_request", "action": "closed"}, nil)
	if expr.Evaluate(e3) {
		t.Error("expected false")
	}
}

func TestBooleanTrue(t *testing.T) {
	expr, err := Parse(`payload.repository.private = true`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(nil, map[string]interface{}{
		"repository": map[string]interface{}{"private": true},
	})
	if !expr.Evaluate(e) {
		t.Error("expected true")
	}
}

func TestBooleanFalse(t *testing.T) {
	expr, err := Parse(`payload.repository.private = false`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(nil, map[string]interface{}{
		"repository": map[string]interface{}{"private": false},
	})
	if !expr.Evaluate(e) {
		t.Error("expected true")
	}
	e2 := makeEvent(nil, map[string]interface{}{
		"repository": map[string]interface{}{"private": true},
	})
	if expr.Evaluate(e2) {
		t.Error("expected false")
	}
}

func TestNullLiteral(t *testing.T) {
	expr, err := Parse(`payload.pull_request = null`)
	if err != nil {
		t.Fatal(err)
	}
	// Field absent → null matches
	e := makeEvent(nil, map[string]interface{}{})
	if !expr.Evaluate(e) {
		t.Error("expected true for absent field")
	}
	// Field present → null does not match
	e2 := makeEvent(nil, map[string]interface{}{
		"pull_request": map[string]interface{}{"id": 1.0},
	})
	if expr.Evaluate(e2) {
		t.Error("expected false for present field")
	}
}

func TestHasTrue(t *testing.T) {
	expr, err := Parse(`has(payload.pull_request.node_id)`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(nil, map[string]interface{}{
		"pull_request": map[string]interface{}{"node_id": "abc123"},
	})
	if !expr.Evaluate(e) {
		t.Error("expected true")
	}
}

func TestHasFalse(t *testing.T) {
	expr, err := Parse(`has(payload.pull_request.node_id)`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(nil, map[string]interface{}{})
	if expr.Evaluate(e) {
		t.Error("expected false for missing field")
	}
}

func TestNestedPayload(t *testing.T) {
	expr, err := Parse(`payload.repository.visibility = "public"`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(nil, map[string]interface{}{
		"repository": map[string]interface{}{"visibility": "public"},
	})
	if !expr.Evaluate(e) {
		t.Error("expected true")
	}
}

func TestTypeMismatch(t *testing.T) {
	// Comparing a string field to a boolean literal → false
	expr, err := Parse(`attributes.gh_event = true`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(map[string]string{"gh_event": "pull_request"}, nil)
	if expr.Evaluate(e) {
		t.Error("expected false for type mismatch")
	}
}

func TestMissingFieldEquality(t *testing.T) {
	expr, err := Parse(`attributes.gh_event = "something"`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(map[string]string{}, nil)
	if expr.Evaluate(e) {
		t.Error("expected false for missing field")
	}
}

func TestFullExpression(t *testing.T) {
	input := `(attributes.gh_event = "pull_request" OR attributes.gh_event = "issues") AND attributes.action = "opened" AND (payload.repository.visibility = "private" OR payload.repository.private = true)`
	expr, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}

	e := makeEvent(
		map[string]string{"gh_event": "pull_request", "action": "opened"},
		map[string]interface{}{
			"repository": map[string]interface{}{"visibility": "private", "private": true},
		},
	)
	if !expr.Evaluate(e) {
		t.Error("expected true for full expression")
	}

	e2 := makeEvent(
		map[string]string{"gh_event": "push", "action": "opened"},
		map[string]interface{}{
			"repository": map[string]interface{}{"visibility": "private", "private": true},
		},
	)
	if expr.Evaluate(e2) {
		t.Error("expected false for non-matching event type")
	}
}

func TestParseError(t *testing.T) {
	_, err := Parse(`attributes.gh_event = `)
	if err == nil {
		t.Error("expected parse error for incomplete expression")
	}

	_, err = Parse(`= "value"`)
	if err == nil {
		t.Error("expected parse error for missing field")
	}

	_, err = Parse(`attributes.gh_event "value"`)
	if err == nil {
		t.Error("expected parse error for missing operator")
	}
}

func TestAndOrPrecedence(t *testing.T) {
	// a = "1" OR b = "2" AND c = "3" should be a = "1" OR (b = "2" AND c = "3")
	expr, err := Parse(`attributes.a = "1" OR attributes.b = "2" AND attributes.c = "3"`)
	if err != nil {
		t.Fatal(err)
	}

	// a=1 alone should match (OR short-circuits)
	e1 := makeEvent(map[string]string{"a": "1"}, nil)
	if !expr.Evaluate(e1) {
		t.Error("expected true: a=1 should satisfy the OR")
	}

	// b=2 AND c=3 should match
	e2 := makeEvent(map[string]string{"b": "2", "c": "3"}, nil)
	if !expr.Evaluate(e2) {
		t.Error("expected true: b=2 AND c=3 should satisfy")
	}

	// b=2 alone should NOT match (AND binds tighter)
	e3 := makeEvent(map[string]string{"b": "2"}, nil)
	if expr.Evaluate(e3) {
		t.Error("expected false: b=2 without c=3 should not satisfy")
	}
}

func TestInequalityWithNull(t *testing.T) {
	expr, err := Parse(`payload.field != null`)
	if err != nil {
		t.Fatal(err)
	}
	// Absent field → != null is false
	e1 := makeEvent(nil, map[string]interface{}{})
	if expr.Evaluate(e1) {
		t.Error("expected false for absent field != null")
	}
	// Present field → != null is true
	e2 := makeEvent(nil, map[string]interface{}{"field": "value"})
	if !expr.Evaluate(e2) {
		t.Error("expected true for present field != null")
	}
}

func TestInequalityMissingField(t *testing.T) {
	expr, err := Parse(`attributes.missing != "something"`)
	if err != nil {
		t.Fatal(err)
	}
	e := makeEvent(map[string]string{}, nil)
	if !expr.Evaluate(e) {
		t.Error("expected true: missing field != 'something' should be true")
	}
}
