package nodelifecycle

import (
	"errors"
	"testing"

	"control-center/internal/corecontracts"
)

func TestLifecycleRollbackAndCancellationEvidenceMatrixFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		from     State
		to       State
		required []EvidenceCheck
	}{
		{
			name: "cancel drain back to ready",
			from: StateDraining,
			to:   StateReady,
			required: []EvidenceCheck{
				CheckOperationCancelled,
				CheckSchedulingEnabled,
				CheckReadinessPassed,
			},
		},
		{
			name: "rollback replacement to maintenance",
			from: StateReplacing,
			to:   StateMaintenance,
			required: []EvidenceCheck{
				CheckOperationCancelled,
				CheckRollbackVerified,
			},
		},
		{
			name: "rollback removal to maintenance",
			from: StateRemoving,
			to:   StateMaintenance,
			required: []EvidenceCheck{
				CheckOperationCancelled,
				CheckRollbackVerified,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule, exists := transitionRuleFor(test.from, test.to)
			if !exists {
				t.Fatalf("transition %s -> %s is not declared", test.from, test.to)
			}
			if rule.typeOf != TransitionDesired {
				t.Fatalf("transition %s -> %s type = %q, want %q", test.from, test.to, rule.typeOf, TransitionDesired)
			}
			assertEvidenceSet(t, rule.requiredChecks, test.required)

			current := lifecycleInState(test.from)
			next, valid := validTransition(current, test.to, rule)
			if err := ValidateTransition(current, next, valid); err != nil {
				t.Fatalf("complete rollback/cancellation evidence rejected: %v", err)
			}

			for index, missing := range rule.requiredChecks {
				t.Run("missing_"+string(missing), func(t *testing.T) {
					request := valid
					request.Evidence.PassedChecks = append([]EvidenceCheck(nil), rule.requiredChecks[:index]...)
					request.Evidence.PassedChecks = append(request.Evidence.PassedChecks, rule.requiredChecks[index+1:]...)
					if err := ValidateTransition(current, next, request); !errors.Is(err, ErrInvalidEvidence) {
						t.Fatalf("ValidateTransition() error = %v, want ErrInvalidEvidence", err)
					}
				})
			}

			wrongType := valid
			wrongType.Type = TransitionObservation
			wrongNext := successor(current, test.to, TransitionObservation)
			wrongType.Precondition = valid.Precondition
			wrongType.To = wrongNext.State
			wrongType.Reason = wrongNext.Reason
			if err := ValidateTransition(current, wrongNext, wrongType); !errors.Is(err, corecontracts.ErrInvalidTransition) {
				t.Fatalf("observation downgrade error = %v, want ErrInvalidTransition", err)
			}
		})
	}
}

func assertEvidenceSet(t *testing.T, got, want []EvidenceCheck) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("required evidence count = %d, want %d: got=%v want=%v", len(got), len(want), got, want)
	}
	gotSet := make(map[EvidenceCheck]struct{}, len(got))
	for _, check := range got {
		gotSet[check] = struct{}{}
	}
	for _, check := range want {
		if _, exists := gotSet[check]; !exists {
			t.Fatalf("required evidence missing %q: got=%v want=%v", check, got, want)
		}
	}
}
