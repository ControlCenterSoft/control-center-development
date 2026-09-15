package action

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"control-center/internal/orchestration/events"
	"control-center/internal/orchestration/policy"
)

type strictTypedActionNestedInput struct {
	Value string `json:"value"`
}

type strictTypedActionInput struct {
	Name   string                         `json:"name"`
	Nested strictTypedActionNestedInput   `json:"nested"`
	Items  []strictTypedActionNestedInput `json:"items"`
}

func TestNewTypedRejectsDuplicateJSONFieldsRecursively(t *testing.T) {
	executed := false
	verified := false
	definition := NewTyped(
		"test.strict-json",
		"test.execute",
		policy.RiskLow,
		json.RawMessage(`{"type":"object"}`),
		func(context.Context, strictTypedActionInput) (events.Output, error) {
			executed = true
			return events.Output{}, nil
		},
		func(context.Context, strictTypedActionInput, events.Output) error {
			verified = true
			return nil
		},
	)

	for _, testCase := range []struct {
		name string
		raw  string
	}{
		{name: "top level", raw: `{"name":"first","name":"second","nested":{"value":"nested"},"items":[]}`},
		{name: "nested object", raw: `{"name":"first","nested":{"value":"first","value":"second"},"items":[]}`},
		{name: "object inside array", raw: `{"name":"first","nested":{"value":"nested"},"items":[{"value":"first","value":"second"}]}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			executed = false
			if _, err := definition.Execute(context.Background(), json.RawMessage(testCase.raw)); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Execute() error = %v, want ErrInvalidInput", err)
			}
			if executed {
				t.Fatal("executor ran for ambiguous duplicate JSON input")
			}
		})
	}

	duplicate := json.RawMessage(`{"name":"first","nested":{"value":"first","value":"second"},"items":[]}`)
	if err := definition.Verify(context.Background(), duplicate, events.Output{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Verify() error = %v, want ErrInvalidInput", err)
	}
	if verified {
		t.Fatal("verifier ran for ambiguous duplicate JSON input")
	}
}

func TestNewTypedPreservesStrictInputBoundary(t *testing.T) {
	executed := false
	definition := NewTyped(
		"test.strict-json",
		"test.execute",
		policy.RiskLow,
		json.RawMessage(`{"type":"object"}`),
		func(context.Context, strictTypedActionInput) (events.Output, error) {
			executed = true
			return events.Output{}, nil
		},
		func(context.Context, strictTypedActionInput, events.Output) error { return nil },
	)

	for _, raw := range []string{
		`{"name":"first","nested":{"value":"nested"},"items":[],"unexpected":true}`,
		`{"name":"first","nested":{"value":"nested"},"items":[]} {}`,
	} {
		executed = false
		if _, err := definition.Execute(context.Background(), json.RawMessage(raw)); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Execute(%q) error = %v, want ErrInvalidInput", raw, err)
		}
		if executed {
			t.Fatal("executor ran for invalid typed action input")
		}
	}

	executed = false
	valid := json.RawMessage(`{"name":"first","nested":{"value":"nested"},"items":[{"value":"item"}]}`)
	if _, err := definition.Execute(context.Background(), valid); err != nil {
		t.Fatalf("Execute(valid) error = %v", err)
	}
	if !executed {
		t.Fatal("executor did not run for valid typed action input")
	}
}
