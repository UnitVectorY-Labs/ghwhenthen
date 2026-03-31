package event

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Event represents a normalized Pub/Sub message.
type Event struct {
	Attributes   map[string]string
	PayloadRaw   []byte
	PayloadBytes []byte
	Payload      map[string]interface{}
	Meta         map[string]string
}

// BuildEvent constructs an Event from raw Pub/Sub data and attributes.
func BuildEvent(data []byte, attributes map[string]string) (*Event, error) {
	if attributes == nil {
		attributes = make(map[string]string)
	}

	meta := make(map[string]string)

	// Determine compression and decompress
	compression := attributes["compression"]
	var decompressed []byte

	switch compression {
	case "", "none":
		meta["compression"] = "none"
		decompressed = data
	case "gzip":
		meta["compression"] = "gzip"
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("gzip decompression failed: %w", err)
		}
		defer r.Close()
		decompressed, err = io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("gzip decompression failed: %w", err)
		}
	case "zstd":
		meta["compression"] = "zstd"
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			return nil, fmt.Errorf("zstd decompression failed: %w", err)
		}
		defer decoder.Close()
		decompressed, err = decoder.DecodeAll(data, nil)
		if err != nil {
			return nil, fmt.Errorf("zstd decompression failed: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported compression: %s", compression)
	}

	// Decode JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(decompressed, &payload); err != nil {
		return nil, fmt.Errorf("JSON decode failed: %w", err)
	}

	// Populate meta from attributes
	if v, ok := attributes["gh_event"]; ok {
		meta["event_type"] = v
	}
	if v, ok := attributes["action"]; ok {
		meta["action"] = v
	}

	return &Event{
		Attributes:   attributes,
		PayloadRaw:   data,
		PayloadBytes: decompressed,
		Payload:      payload,
		Meta:         meta,
	}, nil
}

// GetField resolves a dot-separated path against the event's namespaces.
func (e *Event) GetField(path string) (interface{}, bool) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 0 {
		return nil, false
	}

	namespace := parts[0]
	var remainder string
	if len(parts) == 2 {
		remainder = parts[1]
	}

	switch namespace {
	case "attributes":
		if remainder == "" {
			return nil, false
		}
		v, ok := e.Attributes[remainder]
		return v, ok
	case "meta":
		if remainder == "" {
			return nil, false
		}
		v, ok := e.Meta[remainder]
		return v, ok
	case "payload":
		if remainder == "" {
			return nil, false
		}
		return traverseMap(e.Payload, strings.Split(remainder, "."))
	default:
		return nil, false
	}
}

// traverseMap walks a nested map following the given key segments.
func traverseMap(m map[string]interface{}, keys []string) (interface{}, bool) {
	if len(keys) == 0 {
		return nil, false
	}

	val, ok := m[keys[0]]
	if !ok {
		return nil, false
	}

	if len(keys) == 1 {
		return val, true
	}

	nested, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return traverseMap(nested, keys[1:])
}
