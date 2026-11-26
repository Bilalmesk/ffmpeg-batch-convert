// Package workflow runs safe, typed multi-step media workflows. Steps can chain
// FFmpeg conversion, quality gates, MKV muxing, HLS/DASH packaging and audio
// extraction without invoking a shell.
package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Schema is the current workflow document version.
const Schema = 1

// Workflow is a declarative multi-tool recipe.
type Workflow struct {
	Schema      int    `json:"schema"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Steps       []Step `json:"steps"`
}

// Step is one operation in a workflow.
type Step struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	When    *Condition     `json:"when,omitempty"`
	OnError string         `json:"onError,omitempty"` // fail (default) or continue
	Retries int            `json:"retries,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
}

// Condition is a structured predicate over the workflow context.
type Condition struct {
	Field       string      `json:"field,omitempty"`
	Equals      any         `json:"equals,omitempty"`
	NotEquals   any         `json:"notEquals,omitempty"`
	In          []any       `json:"in,omitempty"`
	Contains    any         `json:"contains,omitempty"`
	Exists      *bool       `json:"exists,omitempty"`
	GreaterThan *float64    `json:"greaterThan,omitempty"`
	LessThan    *float64    `json:"lessThan,omitempty"`
	All         []Condition `json:"all,omitempty"`
	Any         []Condition `json:"any,omitempty"`
	Not         *Condition  `json:"not,omitempty"`
}

// Known step types.
var stepTypes = map[string]bool{
	"probe":   true,
	"convert": true,
	"quality": true,
	"mux":     true,
	"package": true,
	"audio":   true,
	"copy":    true,
	"delete":  true,
}

// Load reads a workflow JSON file. Unknown fields are rejected so typos do not
// silently change a pipeline.
func Load(path string) (Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, err
	}
	return Parse(data)
}

// Parse decodes and validates a workflow document.
func Parse(data []byte) (Workflow, error) {
	var wf Workflow
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wf); err != nil {
		return Workflow{}, fmt.Errorf("parse workflow: %w", err)
	}
	if err := wf.Validate(); err != nil {
		return Workflow{}, err
	}
	return wf, nil
}

// Validate checks the workflow structure and step IDs.
func (w Workflow) Validate() error {
	if w.Schema != 0 && w.Schema != Schema {
		return fmt.Errorf("unsupported workflow schema %d (this build supports %d)", w.Schema, Schema)
	}
	if strings.TrimSpace(w.Name) == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(w.Steps) == 0 {
		return fmt.Errorf("workflow must contain at least one step")
	}
	seen := make(map[string]bool, len(w.Steps))
	for i, step := range w.Steps {
		step.ID = strings.TrimSpace(step.ID)
		if step.ID == "" {
			return fmt.Errorf("step %d: id is required", i+1)
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		seen[step.ID] = true
		if !stepTypes[step.Type] {
			return fmt.Errorf("step %q: unknown type %q", step.ID, step.Type)
		}
		if step.OnError != "" && step.OnError != "fail" && step.OnError != "continue" {
			return fmt.Errorf("step %q: onError must be fail or continue", step.ID)
		}
		if step.Retries < 0 || step.Retries > 10 {
			return fmt.Errorf("step %q: retries must be between 0 and 10", step.ID)
		}
		if step.When != nil {
			if err := step.When.Validate(); err != nil {
				return fmt.Errorf("step %q: %w", step.ID, err)
			}
		}
	}
	return nil
}

// Validate checks a condition tree.
func (c Condition) Validate() error {
	if len(c.All) > 0 && len(c.Any) > 0 {
		return fmt.Errorf("condition cannot use both all and any")
	}
	for i := range c.All {
		if err := c.All[i].Validate(); err != nil {
			return err
		}
	}
	for i := range c.Any {
		if err := c.Any[i].Validate(); err != nil {
			return err
		}
	}
	if c.Not != nil {
		if err := c.Not.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Marshal renders a workflow as indented JSON.
func (w Workflow) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Save writes a workflow atomically.
func (w Workflow) Save(path string) error {
	if err := w.Validate(); err != nil {
		return err
	}
	data, err := w.Marshal()
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(path)
		if retry := os.Rename(temp, path); retry != nil {
			_ = os.Remove(temp)
			return retry
		}
	}
	return nil
}
