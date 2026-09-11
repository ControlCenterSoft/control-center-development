package networkpolicy

import (
	"fmt"
	"time"
)

// BuildCurrentVerificationPreflightAdmission is the orchestration boundary for
// turning verification evidence into a preflight admission. In addition to the
// immutable evidence checks performed by BuildVerificationPreflightAdmission,
// it requires the change machine to still represent the exact authoritative
// ChangePlan supplied by the caller. A desired-state revision change therefore
// invalidates the old machine/evidence path before an admission can be built.
func BuildCurrentVerificationPreflightAdmission(
	now time.Time,
	currentPlan ChangePlan,
	machine *ChangeMachine,
	evidence VerificationEvidence,
	policy ChangePlanVerificationFreshnessPolicy,
) (VerificationPreflightAdmission, error) {
	snapshot, err := validateCurrentVerificationPlan(machine, currentPlan)
	if err != nil {
		return VerificationPreflightAdmission{}, err
	}

	return BuildVerificationPreflightAdmission(now, currentPlan, snapshot, evidence, policy)
}

// ApplyCurrentVerifiedPreflight revalidates the authoritative ChangePlan again
// at the state-transition boundary. This closes the window where desired state
// could change after admission construction but before preflight is advanced.
// The admission remains non-authorizing; normal state-machine version, expiry,
// and transition checks are still enforced by ApplyVerifiedPreflight.
func ApplyCurrentVerifiedPreflight(
	machine *ChangeMachine,
	currentPlan ChangePlan,
	admission VerificationPreflightAdmission,
	at time.Time,
) (ChangeSnapshot, error) {
	snapshot, err := validateCurrentVerificationPlan(machine, currentPlan)
	if err != nil {
		return snapshot, err
	}
	if admission.PlanID != currentPlan.PlanID || admission.RevisionID != currentPlan.RevisionID {
		return snapshot, fmt.Errorf("%w: admission does not match current authoritative plan", ErrInvalidVerificationPreflightAdmission)
	}

	return ApplyVerifiedPreflight(machine, admission, at)
}

func validateCurrentVerificationPlan(machine *ChangeMachine, currentPlan ChangePlan) (ChangeSnapshot, error) {
	if machine == nil {
		return ChangeSnapshot{}, fmt.Errorf("%w: change machine is required", ErrInvalidVerificationPreflightAdmission)
	}

	validated, err := BuildChangePlan(ChangePlanRequest{
		NodeID:     currentPlan.NodeID,
		RevisionID: currentPlan.RevisionID,
		Interfaces: currentPlan.Interfaces,
		Forwarding: currentPlan.Forwarding,
		Probes:     currentPlan.Probes,
		Timeouts:   currentPlan.Timeouts,
	})
	if err != nil {
		return machine.Snapshot(), fmt.Errorf("%w: current authoritative plan is invalid: %v", ErrInvalidVerificationPreflightAdmission, err)
	}
	if validated.PlanID != currentPlan.PlanID || validated.RevisionID != currentPlan.RevisionID {
		return machine.Snapshot(), fmt.Errorf("%w: current authoritative plan identity is not canonical", ErrInvalidVerificationPreflightAdmission)
	}

	snapshot := machine.Snapshot()
	if snapshot.PlanID != currentPlan.PlanID {
		return snapshot, fmt.Errorf("%w: change machine no longer matches current authoritative plan", ErrInvalidVerificationPreflightAdmission)
	}
	return snapshot, nil
}
