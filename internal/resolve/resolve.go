package resolve

import (
	"fmt"
	"strings"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/event"
)

// ResolveContext holds all data needed for variable resolution.
type ResolveContext struct {
	Event     *event.Event
	Constants map[string]interface{}
	Steps     map[string]map[string]interface{}
}

// Resolve resolves a string value that may contain ${...} references.
// If the entire string is a single ${...} reference, the raw resolved value is returned.
// Otherwise, references are interpolated as strings into the surrounding text.
func Resolve(input string, ctx *ResolveContext) (interface{}, error) {
	refs, err := parseReferences(input)
	if err != nil {
		return nil, err
	}

	if len(refs) == 0 {
		return input, nil
	}

	// If the entire string is exactly one reference with no surrounding text, return raw value.
	if len(refs) == 1 && refs[0].start == 0 && refs[0].end == len(input) {
		return resolveReference(refs[0].expr, ctx)
	}

	// Mixed string: resolve each reference and interpolate as string.
	var b strings.Builder
	prev := 0
	for _, ref := range refs {
		b.WriteString(input[prev:ref.start])
		val, err := resolveReference(ref.expr, ctx)
		if err != nil {
			return nil, err
		}
		b.WriteString(fmt.Sprintf("%v", val))
		prev = ref.end
	}
	b.WriteString(input[prev:])
	return b.String(), nil
}

// ResolveString resolves a string value and converts the result to a string.
func ResolveString(input string, ctx *ResolveContext) (string, error) {
	val, err := Resolve(input, ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", val), nil
}

// ResolveMap resolves all values in a map[string]string and returns a map[string]interface{}.
func ResolveMap(m map[string]string, ctx *ResolveContext) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		val, err := Resolve(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("resolving key %q: %w", k, err)
		}
		result[k] = val
	}
	return result, nil
}

// reference represents a parsed ${...} token in the input string.
type reference struct {
	start int // index of '$'
	end   int // index after closing '}'
	expr  string
}

// parseReferences finds all ${...} patterns in the input.
func parseReferences(input string) ([]reference, error) {
	var refs []reference
	i := 0
	for i < len(input) {
		idx := strings.Index(input[i:], "${")
		if idx < 0 {
			break
		}
		start := i + idx
		// Find matching closing brace, accounting for nested parens in coalesce.
		depth := 0
		j := start + 2
		found := false
		for j < len(input) {
			switch input[j] {
			case '(':
				depth++
			case ')':
				depth--
			case '}':
				if depth == 0 {
					refs = append(refs, reference{
						start: start,
						end:   j + 1,
						expr:  input[start+2 : j],
					})
					found = true
				}
			}
			if found {
				break
			}
			j++
		}
		if !found {
			return nil, fmt.Errorf("unclosed variable reference starting at position %d", start)
		}
		i = j + 1
	}
	return refs, nil
}

// resolveReference resolves a single expression (the content inside ${...}).
func resolveReference(expr string, ctx *ResolveContext) (interface{}, error) {
	expr = strings.TrimSpace(expr)

	if strings.HasPrefix(expr, "coalesce(") && strings.HasSuffix(expr, ")") {
		return resolveCoalesce(expr, ctx)
	}

	return resolvePath(expr, ctx)
}

// resolvePath resolves a single dotted path reference.
func resolvePath(path string, ctx *ResolveContext) (interface{}, error) {
	path = strings.TrimSpace(path)

	switch {
	case strings.HasPrefix(path, "constants."):
		return resolveConstants(path[len("constants."):], ctx.Constants)

	case strings.HasPrefix(path, "steps."):
		return resolveSteps(path[len("steps."):], ctx.Steps)

	case strings.HasPrefix(path, "attributes.") ||
		strings.HasPrefix(path, "payload.") ||
		strings.HasPrefix(path, "meta."):
		if ctx.Event == nil {
			return nil, fmt.Errorf("cannot resolve %q: no event in context", path)
		}
		val, ok := ctx.Event.GetField(path)
		if !ok {
			return nil, fmt.Errorf("cannot resolve %q: field not found", path)
		}
		return val, nil

	default:
		return nil, fmt.Errorf("unknown reference namespace in %q", path)
	}
}

// resolveCoalesce implements coalesce(a, b, ...) returning the first non-nil value.
func resolveCoalesce(expr string, ctx *ResolveContext) (interface{}, error) {
	inner := expr[len("coalesce(") : len(expr)-1]
	args := strings.Split(inner, ",")

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		val, err := resolvePath(arg, ctx)
		if err == nil && val != nil {
			return val, nil
		}
	}
	return nil, fmt.Errorf("coalesce: all arguments resolved to nil")
}

// resolveConstants traverses a nested map using a dot-separated path.
func resolveConstants(path string, constants map[string]interface{}) (interface{}, error) {
	keys := strings.Split(path, ".")
	var current interface{} = constants

	for _, key := range keys {
		switch m := current.(type) {
		case map[string]interface{}:
			val, ok := m[key]
			if !ok {
				return nil, fmt.Errorf("cannot resolve constants path %q: key %q not found", path, key)
			}
			current = val
		case map[interface{}]interface{}:
			val, ok := m[key]
			if !ok {
				return nil, fmt.Errorf("cannot resolve constants path %q: key %q not found", path, key)
			}
			current = val
		default:
			return nil, fmt.Errorf("cannot resolve constants path %q: value at %q is not a map", path, key)
		}
	}
	return current, nil
}

// resolveSteps resolves steps.<step_name>.outputs.<output_name>.
func resolveSteps(path string, steps map[string]map[string]interface{}) (interface{}, error) {
	parts := strings.SplitN(path, ".", 3)
	if len(parts) < 3 || parts[1] != "outputs" {
		return nil, fmt.Errorf("invalid steps reference %q: expected steps.<name>.outputs.<key>", "steps."+path)
	}

	stepName := parts[0]
	outputKey := parts[2]

	stepOutputs, ok := steps[stepName]
	if !ok {
		return nil, fmt.Errorf("cannot resolve steps reference: step %q not found", stepName)
	}

	val, ok := stepOutputs[outputKey]
	if !ok {
		return nil, fmt.Errorf("cannot resolve steps reference: output %q not found for step %q", outputKey, stepName)
	}
	return val, nil
}
