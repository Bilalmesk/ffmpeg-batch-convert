package workflow

import (
	"strings"
	"testing"
)

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte(`{"schema":1,"name":"x","steps":[{"id":"a","type":"probe","typo":true}]}`))
	if err == nil {
		t.Fatal("expected an unknown-field error")
	}
}

func TestValidateRejectsDuplicateAndUnknown(t *testing.T) {
	wf := Workflow{Name: "x", Steps: []Step{{ID: "a", Type: "probe"}, {ID: "a", Type: "probe"}}}
	if err := wf.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %v", err)
	}
	wf = Workflow{Name: "x", Steps: []Step{{ID: "a", Type: "nope"}}}
	if err := wf.Validate(); err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuiltinsValidate(t *testing.T) {
	for _, wf := range Builtins() {
		if err := wf.Validate(); err != nil {
			t.Fatalf("%s: %v", wf.Name, err)
		}
	}
	if _, ok := Builtin("multi-delivery"); !ok {
		t.Fatal("multi-delivery should exist")
	}
}

func TestResolveTemplates(t *testing.T) {
	ctx := map[string]any{
		"input":     `C:\in.ts`,
		"base":      "in",
		"outputDir": `C:\out`,
		"steps": map[string]any{
			"encode": map[string]any{"output": `C:\work\in.mp4`, "score": 95.5},
		},
	}
	got, err := resolveString("{{outputDir}}/{{base}}.mp4", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != `C:\out/in.mp4` {
		t.Fatalf("resolved = %q", got)
	}
	raw, err := resolveString("{{steps.encode.score}}", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if number, ok := raw.(float64); !ok || number != 95.5 {
		t.Fatalf("raw = %#v", raw)
	}
	if _, err := resolveString("{{missing}}", ctx); err == nil {
		t.Fatal("expected an undefined template error")
	}
}

func TestConditions(t *testing.T) {
	ctx := map[string]any{
		"ext": "ts",
		"probe": map[string]any{
			"videoCodec":  "hevc",
			"audioTracks": 2,
			"audioCodecs": []string{"aac", "opus"},
		},
	}
	tests := []struct {
		condition Condition
		want      bool
	}{
		{Condition{Field: "ext", Equals: "TS"}, true},
		{Condition{Field: "ext", In: []any{"ts", "m2ts"}}, true},
		{Condition{Field: "probe.videoCodec", Equals: "h264"}, false},
		{Condition{Field: "probe.audioTracks", GreaterThan: floatPtr(1)}, true},
		{Condition{Field: "probe.audioCodecs", Contains: "opus"}, true},
		{Condition{All: []Condition{{Field: "ext", Equals: "ts"}, {Field: "probe.audioTracks", Equals: 2}}}, true},
		{Condition{Any: []Condition{{Field: "ext", Equals: "mp4"}, {Field: "probe.audioTracks", Equals: 2}}}, true},
		{Condition{Not: &Condition{Field: "ext", Equals: "mp4"}}, true},
	}
	for i, test := range tests {
		got, err := test.condition.Eval(ctx)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if got != test.want {
			t.Fatalf("case %d: got %v, want %v", i, got, test.want)
		}
	}
}

func floatPtr(value float64) *float64 { return &value }
