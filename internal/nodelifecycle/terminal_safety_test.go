package nodelifecycle

import (
	"errors"
	"testing"
)

func TestTerminalLifecycleTransitionsRequireEveryDeclaredSafetyCheck(t *testing.T) {
	tests := []struct {
		from State
		to   State
	}{
		{from: StateReplacing, to: StateRetired},
		{from: StateRemoving, to: StateRetired},
		{from: StateOffline, to: StateRetired},
	}

	for _, test := range tests {
		t.Run(string(test.from)+"_to_"+string(test.to), func(t *testing.T) {
			rule, ok := transitionRuleFor(test.from, test.to)
			if !ok {
				t.Fatalf("missing declared transition %s -> %s", test.from, test.to)
			}
			if len(rule.requiredChecks) == 0 {
				t.Fatalf("terminal transition %s -> %s has no safety evidence", test.from, test.to)
			}

			current := lifecycleInState(test.from)
			next, valid := validTransition(current, test.to, rule)

			for index, missing := range rule.requiredChecks {
				t.Run("missing_"+string(missing), func(t *testing.T) {
					request := valid
					request.Evidence.PassedChecks = append([]EvidenceCheck(nil), valid.Evidence.PassedChecks[:index]...)
					request.Evidence.PassedChecks = append(request.Evidence.PassedChecks, valid.Evidence.PassedChecks[index+1:]...)

					if err := ValidateTransition(current, next, request); !errors.Is(err, ErrInvalidEvidence) {
						t.Fatalf("missing %q error = %v, want ErrInvalidEvidence", missing, err)
					}
				})
			}
		})
	}
}
