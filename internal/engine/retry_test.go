package engine

import (
	"context"
	"errors"
	"testing"
)

func TestFailedInputs(t *testing.T) {
	run := &Run{Jobs: []*Job{
		{Input: "a.ts", Status: StatusFailed},
		{Input: "b.ts", Status: StatusDone},
		{Input: "a.ts", Status: StatusFailed},
		{Input: "c.ts", Status: StatusCanceled},
	}}
	files := failedInputs(run)
	if len(files) != 1 || files[0] != "a.ts" {
		t.Fatalf("failedInputs = %v, want [a.ts]", files)
	}
}

func TestRetryUnknownRun(t *testing.T) {
	manager := &Manager{history: []*Run{}}
	if _, err := manager.Retry(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Retry error = %v, want ErrNotFound", err)
	}
}

func TestRetryWithoutFailures(t *testing.T) {
	manager := &Manager{history: []*Run{{
		ID:   "run-ok",
		Jobs: []*Job{{Input: "a.ts", Status: StatusDone}},
	}}}
	if _, err := manager.Retry(context.Background(), "run-ok"); !errors.Is(err, ErrNoFailures) {
		t.Fatalf("Retry error = %v, want ErrNoFailures", err)
	}
}
