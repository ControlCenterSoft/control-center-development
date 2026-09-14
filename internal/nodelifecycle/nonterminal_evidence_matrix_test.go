package nodelifecycle

import (
	"errors"
	"testing"
)

func TestEveryNonTerminalSafetyTransitionRequiresEachEvidenceCheck(t *testing.T) {
	coveredTransitions := 0
	coveredChecks := 0

	for from, targets := range transitionRules {
		for to, rule := range targets {
			if to == StateRetired || len(rule.requiredChecks) == 0 {
				continue
			}
			coveredTransitions++

			current := lifecycleInState(from)
			next, valid := validTransition(current, to, rule)

			for index, missing := range rule.requiredChecks {
				coveredChecks++
				t.Run(string(from)+"_to_"+string(to)+"_missing_"+string(missing), func(t *testing.T) {
					request := valid
					request.Evidence.PassedChecks = append([]EvidenceCheck(nil), rule.requiredChecks[:index]...)
					request.Evidence.PassedChecks = append(request.Evidence.PassedChecks, rule.requiredChecks[index+1:]...)

					if err := ValidateTransition(current, next, request); !errors.Is(err, ErrInvalidEvidence) {
						t.Fatalf("ValidateTransition() error = %v, want ErrInvalidEvidence", err)
					}
				})
			}
		}
	}

	if coveredTransitions == 0 || coveredChecks == 0 {
		t.Fatalf("non-terminal safety evidence matrix is unexpectedly empty: transitions=%d checks=%d", coveredTransitions, coveredChecks)
	}
}
