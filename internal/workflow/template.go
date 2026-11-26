package workflow

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var templatePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

type runState struct {
	Input      string
	OutputDir  string
	WorkDir    string
	Base       string
	Ext        string
	Overwrite  bool
	ToolPaths  map[string]string
	Probe      any
	Steps      map[string]*stepResult
	LastOutput string
	Vars       map[string]any
}

type stepResult struct {
	Output   string
	Path     string
	Manifest string
	Score    float64
	Info     any
	Values   map[string]any
}

func buildContext(state *runState) map[string]any {
	ctx := map[string]any{
		"input":     state.Input,
		"name":      filepath.Base(state.Input),
		"base":      state.Base,
		"ext":       state.Ext,
		"work":      state.WorkDir,
		"outputDir": state.OutputDir,
		"overwrite": state.Overwrite,
	}
	if probe, ok := state.Probe.(map[string]any); ok {
		ctx["probe"] = probe
	}
	steps := make(map[string]any, len(state.Steps))
	for id, result := range state.Steps {
		entry := map[string]any{
			"output":   result.Output,
			"path":     result.Path,
			"manifest": result.Manifest,
			"score":    result.Score,
		}
		for key, value := range result.Values {
			entry[key] = value
		}
		steps[id] = entry
	}
	ctx["steps"] = steps
	for key, value := range state.Vars {
		ctx[key] = value
	}
	return ctx
}

// resolveParams resolves templates recursively in a step parameter map.
func resolveParams(params map[string]any, ctx map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(params))
	for key, value := range params {
		resolved, err := resolveValue(value, ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = resolved
	}
	return out, nil
}

func resolveValue(value any, ctx map[string]any) (any, error) {
	switch typed := value.(type) {
	case string:
		return resolveString(typed, ctx)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved, err := resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			resolved, err := resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return value, nil
	}
}

func resolveString(value string, ctx map[string]any) (any, error) {
	matches := templatePattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value, nil
	}
	// A parameter that is exactly one template keeps the referenced type.
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(value) {
		key := value[matches[0][2]:matches[0][3]]
		found, ok := lookup(ctx, key)
		if !ok {
			return nil, fmt.Errorf("template %q is not defined", key)
		}
		return found, nil
	}
	var builder strings.Builder
	last := 0
	for _, match := range matches {
		builder.WriteString(value[last:match[0]])
		key := value[match[2]:match[3]]
		found, ok := lookup(ctx, key)
		if !ok {
			return nil, fmt.Errorf("template %q is not defined", key)
		}
		builder.WriteString(stringify(found))
		last = match[1]
	}
	builder.WriteString(value[last:])
	return builder.String(), nil
}

func lookup(ctx map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = ctx
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = value
		default:
			return nil, false
		}
	}
	return current, true
}

func stringify(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case []string:
		return strings.Join(typed, ",")
	default:
		return fmt.Sprint(typed)
	}
}

func (c Condition) Eval(ctx map[string]any) (bool, error) {
	if len(c.All) > 0 {
		for i := range c.All {
			ok, err := c.All[i].Eval(ctx)
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	}
	if len(c.Any) > 0 {
		for i := range c.Any {
			ok, err := c.Any[i].Eval(ctx)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	if c.Not != nil {
		ok, err := c.Not.Eval(ctx)
		return !ok, err
	}
	if strings.TrimSpace(c.Field) == "" {
		return true, nil
	}
	value, exists := lookup(ctx, c.Field)
	if c.Exists != nil {
		if *c.Exists != exists {
			return false, nil
		}
		if !exists {
			return true, nil
		}
	}
	if !exists {
		return false, nil
	}
	if c.Equals != nil {
		return equalValue(value, c.Equals), nil
	}
	if c.NotEquals != nil {
		return !equalValue(value, c.NotEquals), nil
	}
	if len(c.In) > 0 {
		for _, candidate := range c.In {
			if equalValue(value, candidate) {
				return true, nil
			}
		}
		return false, nil
	}
	if c.Contains != nil {
		return containsValue(value, c.Contains), nil
	}
	if c.GreaterThan != nil || c.LessThan != nil {
		number, ok := numberValue(value)
		if !ok {
			return false, nil
		}
		if c.GreaterThan != nil && number <= *c.GreaterThan {
			return false, nil
		}
		if c.LessThan != nil && number >= *c.LessThan {
			return false, nil
		}
		return true, nil
	}
	return true, nil
}

func equalValue(a, b any) bool {
	if left, ok := numberValue(a); ok {
		if right, ok := numberValue(b); ok {
			return left == right
		}
	}
	if left, ok := a.(bool); ok {
		if right, ok := b.(bool); ok {
			return left == right
		}
	}
	return strings.EqualFold(stringify(a), stringify(b))
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case string:
		number, err := strconv.ParseFloat(typed, 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func containsValue(value, needle any) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(strings.ToLower(typed), strings.ToLower(stringify(needle)))
	case []string:
		for _, item := range typed {
			if equalValue(item, needle) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if equalValue(item, needle) {
				return true
			}
		}
	}
	return false
}
