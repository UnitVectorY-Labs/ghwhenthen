package event

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestBuildEvent_Uncompressed(t *testing.T) {
	payload := `{"repository":{"visibility":"public"},"action":"opened"}`
	attrs := map[string]string{"gh_event": "pull_request", "action": "opened"}

	ev, err := BuildEvent([]byte(payload), attrs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(ev.PayloadRaw) != payload {
		t.Errorf("PayloadRaw mismatch")
	}
	if string(ev.PayloadBytes) != payload {
		t.Errorf("PayloadBytes mismatch")
	}
	if ev.Meta["compression"] != "none" {
		t.Errorf("expected compression=none, got %q", ev.Meta["compression"])
	}
	if ev.Meta["event_type"] != "pull_request" {
		t.Errorf("expected event_type=pull_request, got %q", ev.Meta["event_type"])
	}
	if ev.Meta["action"] != "opened" {
		t.Errorf("expected action=opened, got %q", ev.Meta["action"])
	}
	if ev.Payload["action"] != "opened" {
		t.Errorf("expected payload action=opened")
	}
}

func TestBuildEvent_Gzip(t *testing.T) {
	payload := `{"status":"ok"}`

	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	attrs := map[string]string{"compression": "gzip"}
	ev, err := BuildEvent(buf.Bytes(), attrs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(ev.PayloadBytes) != payload {
		t.Errorf("PayloadBytes mismatch: got %q", string(ev.PayloadBytes))
	}
	if ev.Meta["compression"] != "gzip" {
		t.Errorf("expected compression=gzip, got %q", ev.Meta["compression"])
	}
	if ev.Payload["status"] != "ok" {
		t.Errorf("expected status=ok")
	}
}

func TestBuildEvent_Zstd(t *testing.T) {
	payload := `{"level":"info"}`

	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd encoder: %v", err)
	}
	compressed := encoder.EncodeAll([]byte(payload), nil)
	encoder.Close()

	attrs := map[string]string{"compression": "zstd"}
	ev, err := BuildEvent(compressed, attrs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(ev.PayloadBytes) != payload {
		t.Errorf("PayloadBytes mismatch: got %q", string(ev.PayloadBytes))
	}
	if ev.Meta["compression"] != "zstd" {
		t.Errorf("expected compression=zstd, got %q", ev.Meta["compression"])
	}
	if ev.Payload["level"] != "info" {
		t.Errorf("expected level=info")
	}
}

func TestBuildEvent_UnsupportedCompression(t *testing.T) {
	attrs := map[string]string{"compression": "lz4"}
	_, err := BuildEvent([]byte(`{}`), attrs)
	if err == nil {
		t.Fatal("expected error for unsupported compression")
	}
	if got := err.Error(); got != "unsupported compression: lz4" {
		t.Errorf("unexpected error message: %q", got)
	}
}

func TestBuildEvent_InvalidJSON(t *testing.T) {
	_, err := BuildEvent([]byte(`not json`), nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestBuildEvent_GzipDecompressionFails(t *testing.T) {
	attrs := map[string]string{"compression": "gzip"}
	_, err := BuildEvent([]byte("not gzip data"), attrs)
	if err == nil {
		t.Fatal("expected error for invalid gzip data")
	}
}

func TestBuildEvent_ZstdDecompressionFails(t *testing.T) {
	attrs := map[string]string{"compression": "zstd"}
	_, err := BuildEvent([]byte("not zstd data"), attrs)
	if err == nil {
		t.Fatal("expected error for invalid zstd data")
	}
}

func TestGetField_Attributes(t *testing.T) {
	ev := &Event{
		Attributes: map[string]string{"gh_event": "push"},
		Payload:    map[string]interface{}{},
		Meta:       map[string]string{},
	}

	v, ok := ev.GetField("attributes.gh_event")
	if !ok || v != "push" {
		t.Errorf("expected (push, true), got (%v, %v)", v, ok)
	}
}

func TestGetField_Meta(t *testing.T) {
	ev := &Event{
		Attributes: map[string]string{},
		Payload:    map[string]interface{}{},
		Meta:       map[string]string{"compression": "gzip"},
	}

	v, ok := ev.GetField("meta.compression")
	if !ok || v != "gzip" {
		t.Errorf("expected (gzip, true), got (%v, %v)", v, ok)
	}
}

func TestGetField_PayloadNested(t *testing.T) {
	payload := `{"repository":{"visibility":"private","owner":{"login":"octocat"}}}`
	var p map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		t.Fatal(err)
	}

	ev := &Event{
		Attributes: map[string]string{},
		Payload:    p,
		Meta:       map[string]string{},
	}

	tests := []struct {
		path string
		want interface{}
	}{
		{"payload.repository.visibility", "private"},
		{"payload.repository.owner.login", "octocat"},
	}

	for _, tt := range tests {
		v, ok := ev.GetField(tt.path)
		if !ok {
			t.Errorf("GetField(%q): not found", tt.path)
			continue
		}
		if v != tt.want {
			t.Errorf("GetField(%q) = %v, want %v", tt.path, v, tt.want)
		}
	}
}

func TestGetField_MissingFields(t *testing.T) {
	ev := &Event{
		Attributes: map[string]string{},
		Payload:    map[string]interface{}{},
		Meta:       map[string]string{},
	}

	paths := []string{
		"attributes.nonexistent",
		"payload.missing.deep.path",
		"meta.unknown",
		"unknown_namespace.key",
		"",
	}

	for _, p := range paths {
		_, ok := ev.GetField(p)
		if ok {
			t.Errorf("GetField(%q): expected false, got true", p)
		}
	}
}
