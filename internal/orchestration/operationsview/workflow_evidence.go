package operationsview

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"control-center/internal/orchestration/events"
	"control-center/internal/orchestration/job"
)

const (
	OperationsWorkflowEvidenceContractVersion     = "ui.operations-workflow-evidence/v1"
	MaxOperationsWorkflowEvidenceIdentifierLength = 255
)

var ErrInvalidOperationsWorkflowEvidence = errors.New("invalid operations workflow evidence")

type OperationsWorkflowEvidenceState string

const (
	OperationsWorkflowEvidenceComplete OperationsWorkflowEvidenceState = "complete"
	OperationsWorkflowEvidenceBlocked  OperationsWorkflowEvidenceState = "blocked"
)

type OperationsWorkflowBlockReason string

const (
	OperationsWorkflowBlockApproval          OperationsWorkflowBlockReason = "approval_not_satisfied"
	OperationsWorkflowBlockRecovery          OperationsWorkflowBlockReason = "recovery_not_ready"
	OperationsWorkflowBlockVerification      OperationsWorkflowBlockReason = "post_condition_not_verified"
	OperationsWorkflowBlockAudit             OperationsWorkflowBlockReason = "audit_evidence_missing"
)

// OperationsWorkflowEvidence binds the already-built approval, terminal Job
// result and recovery-path projections for one exact immutable Change revision.
// "complete" means only that the evidence chain is internally consistent and
// contains the required safety evidence; it never means that the operation
// itself succeeded and it never grants execution authority.
type OperationsWorkflowEvidence struct {
	ContractVersion           string                          `json:"contract_version"`
	ChangeID                  string                          `json:"change_id"`
	RevisionID                string                          `json:"revision_id"`
	RevisionDigest            string                          `json:"revision_digest"`
	ApprovalState             ApprovalEvidenceState           `json:"approval_state"`
	ApprovalObservedAt        time.Time                       `json:"approval_observed_at"`
	JobID                     string                          `json:"job_id"`
	JobVersion                uint64                          `json:"job_version"`
	Outcome                   job.Status                      `json:"outcome"`
	ResultOutputDigest        string                          `json:"result_output_digest,omitempty"`
	ResultObservedAt          time.Time                       `json:"result_observed_at"`
	RecoveryPointID           string                          `json:"recovery_point_id"`
	RecoveryState             RecoveryPathState               `json:"recovery_state"`
	RecoveryEvaluatedAt       time.Time                       `json:"recovery_evaluated_at"`
	State                     OperationsWorkflowEvidenceState `json:"state"`
	BlockReasons              []OperationsWorkflowBlockReason `json:"block_reasons"`
	EvidenceComplete          bool                            `json:"evidence_complete"`
	ObservedAt                time.Time                       `json:"observed_at"`
	ExecutionAuthorized       bool                            `json:"execution_authorized"`
	ProductionMutationAllowed bool                            `json:"production_mutation_allowed"`
}

type OperationsWorkflowEvidenceInput struct {
	Approval   ApprovalEvidence
	Result     JobResultEvidence
	Recovery   RecoveryPathEvidence
	ObservedAt time.Time
}

// BuildOperationsWorkflowEvidence performs the cross-contract checks that are
// otherwise easy to miss when the operator UI receives the three evidence
// projections independently. Legitimate incomplete safety evidence is reported
// as a bounded blocked state; identity/digest/timestamp contradictions are
// rejected fail-closed.
func BuildOperationsWorkflowEvidence(input OperationsWorkflowEvidenceInput) (OperationsWorkflowEvidence, error) {
	if input.ObservedAt.IsZero() {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("observed_at is required")
	}
	observedAt := input.ObservedAt.UTC()

	approval := input.Approval
	result := input.Result
	recovery := input.Recovery

	if approval.ContractVersion != ApprovalEvidenceContractVersion {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("approval contract_version is invalid")
	}
	if result.ContractVersion != JobResultEvidenceContractVersion {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("result contract_version is invalid")
	}
	if err := ValidateRecoveryPathEvidence(recovery); err != nil {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("recovery evidence is invalid")
	}
	if recovery.ContractVersion != RecoveryPathEvidenceContractVersion {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("recovery contract_version is invalid")
	}

	if err := validateOperationsWorkflowIdentifier("change_id", approval.ChangeID); err != nil {
		return OperationsWorkflowEvidence{}, err
	}
	if err := validateOperationsWorkflowIdentifier("revision_id", approval.RevisionID); err != nil {
		return OperationsWorkflowEvidence{}, err
	}
	if err := validateOperationsWorkflowIdentifier("job_id", result.JobID); err != nil {
		return OperationsWorkflowEvidence{}, err
	}
	if err := validateOperationsWorkflowIdentifier("recovery_point_id", recovery.RecoveryPointID); err != nil {
		return OperationsWorkflowEvidence{}, err
	}
	if !validOperationsWorkflowDigest(approval.RevisionDigest) {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("revision_digest is invalid")
	}
	if result.JobVersion == 0 {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("job_version is required")
	}
	if !result.Outcome.Terminal() {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("result outcome is not terminal")
	}
	if approval.ObservedAt.IsZero() || result.ObservedAt.IsZero() || recovery.EvaluatedAt.IsZero() {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("component observation time is required")
	}
	if approval.ObservedAt.After(observedAt) || result.ObservedAt.After(observedAt) || recovery.EvaluatedAt.After(observedAt) {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("component evidence is newer than workflow observation")
	}
	if approval.ObservedAt.After(result.ObservedAt) {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("approval evidence is newer than terminal result evidence")
	}

	if approval.ChangeID != result.ChangeID || approval.ChangeID != recovery.ChangeID {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("change_id mismatch across evidence")
	}
	if approval.RevisionID != result.RevisionID || approval.RevisionID != recovery.RevisionID {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("revision_id mismatch across evidence")
	}
	if approval.RevisionDigest != result.RevisionDigest || approval.RevisionDigest != recovery.RevisionDigest {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("revision_digest mismatch across evidence")
	}

	if result.OutputPresent {
		if !validOperationsWorkflowDigest(result.OutputDigest) {
			return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("result output_digest is invalid")
		}
	} else if result.Outcome != job.StatusCancelled {
		return OperationsWorkflowEvidence{}, invalidOperationsWorkflow("terminal non-cancelled Job is missing result output")
	}

	blockReasons := make([]OperationsWorkflowBlockReason, 0, 4)
	if !approval.Satisfied || (approval.State != ApprovalEvidenceSatisfied && approval.State != ApprovalEvidenceNotRequired) {
		blockReasons = append(blockReasons, OperationsWorkflowBlockApproval)
	}
	if recovery.State != RecoveryPathReady {
		blockReasons = append(blockReasons, OperationsWorkflowBlockRecovery)
	}
	if result.Outcome == job.StatusSucceeded && (result.HealthChecks == 0 || result.WorstHealth != events.HealthHealthy) {
		blockReasons = append(blockReasons, OperationsWorkflowBlockVerification)
	}
	if result.AuditEvents == 0 {
		blockReasons = append(blockReasons, OperationsWorkflowBlockAudit)
	}

	state := OperationsWorkflowEvidenceComplete
	if len(blockReasons) != 0 {
		state = OperationsWorkflowEvidenceBlocked
	}

	return OperationsWorkflowEvidence{
		ContractVersion:           OperationsWorkflowEvidenceContractVersion,
		ChangeID:                  approval.ChangeID,
		RevisionID:                approval.RevisionID,
		RevisionDigest:            approval.RevisionDigest,
		ApprovalState:             approval.State,
		ApprovalObservedAt:        approval.ObservedAt.UTC(),
		JobID:                     result.JobID,
		JobVersion:                result.JobVersion,
		Outcome:                   result.Outcome,
		ResultOutputDigest:        result.OutputDigest,
		ResultObservedAt:          result.ObservedAt.UTC(),
		RecoveryPointID:           recovery.RecoveryPointID,
		RecoveryState:             recovery.State,
		RecoveryEvaluatedAt:       recovery.EvaluatedAt.UTC(),
		State:                     state,
		BlockReasons:              blockReasons,
		EvidenceComplete:          state == OperationsWorkflowEvidenceComplete,
		ObservedAt:                observedAt,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}, nil
}

func validateOperationsWorkflowIdentifier(name, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value || len(value) > MaxOperationsWorkflowEvidenceIdentifierLength {
		return invalidOperationsWorkflow("canonical " + name + " is required")
	}
	return nil
}

func validOperationsWorkflowDigest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, ch := range value[len("sha256:"):] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func invalidOperationsWorkflow(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidOperationsWorkflowEvidence, message)
}
