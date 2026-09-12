package incidents

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	CorrelationContractV1 = "incidents.correlation-decision/v1"
	maxCorrelationInputs  = 256
)

var (
	ErrInvalidCorrelationObservation = errors.New("invalid incident correlation observation")
	ErrInvalidCorrelationDependency  = errors.New("invalid incident correlation dependency")
)

type CorrelationState string

const (
	CorrelationMatch         CorrelationState = "match"
	CorrelationAlreadyLinked CorrelationState = "already-linked"
	CorrelationNewCandidate  CorrelationState = "new-candidate"
	CorrelationBlocked       CorrelationState = "blocked"
)

const (
	CorrelationBlockerAmbiguousExactMatch = "multiple_exact_open_incidents"
	CorrelationBlockerSignalIDConflict    = "signal_id_linked_to_multiple_incidents"
)

// CorrelationObservation is a bounded, already-redacted signal envelope used
// only to decide whether a newly observed signal can be linked to one existing
// incident. Correlation is deliberately strict: it requires exact scope,
// signal kind/source and affected-resource identity rather than fuzzy text
// similarity. Summary/evidence are validated, but are never hashed into the
// correlation fingerprint.
type CorrelationObservation struct {
	ScopeID           string        `json:"scope_id"`
	Signal            Signal        `json:"signal"`
	AffectedResources []ResourceRef `json:"affected_resources"`
}

// CorrelationDecision is read-only evidence. It never creates, acknowledges,
// resolves or otherwise mutates an incident. A caller that wants to persist a
// decision must pass through a separately authorized mutation boundary.
type CorrelationDecision struct {
	Contract                  string           `json:"contract"`
	State                     CorrelationState `json:"state"`
	Fingerprint               string           `json:"fingerprint"`
	IncidentID                string           `json:"incident_id,omitempty"`
	IncidentResourceVersion   string           `json:"incident_resource_version,omitempty"`
	IncidentGeneration        uint64           `json:"incident_generation,omitempty"`
	Blockers                  []string         `json:"blockers,omitempty"`
	ExecutionAuthorized       bool             `json:"execution_authorized"`
	ProductionMutationAllowed bool             `json:"production_mutation_allowed"`
}

// EvaluateCorrelation performs a deterministic, fail-closed correlation pass
// over a bounded set of authoritative incident read models.
//
// Exact matching requires:
//   - the same server-side scope;
//   - an open/acknowledged incident (resolved incidents are historical only);
//   - exactly the same affected resource set, independent of input ordering;
//   - at least one existing signal with the same kind and source;
//   - the new observation not predating the incident.
//
// Duplicate signal IDs are treated specially: one exact existing owner returns
// already-linked, while more than one owner is a hard ambiguity. Any invalid
// stored incident fails the whole evaluation rather than being silently
// ignored and potentially producing a false "new candidate" result.
func EvaluateCorrelation(observation CorrelationObservation, candidates []Incident) (CorrelationDecision, error) {
	if err := validateCorrelationObservation(observation); err != nil {
		return CorrelationDecision{}, err
	}
	if len(candidates) > maxCorrelationInputs {
		return CorrelationDecision{}, fmt.Errorf("%w: candidate count exceeds %d", ErrInvalidCorrelationDependency, maxCorrelationInputs)
	}

	fingerprint := correlationFingerprint(observation)
	exactMatches := make([]Incident, 0, 1)
	linkedOwners := make([]Incident, 0, 1)

	for idx := range candidates {
		candidate := candidates[idx]
		if err := candidate.Validate(); err != nil {
			return CorrelationDecision{}, fmt.Errorf("%w: candidates[%d] failed validation: %v", ErrInvalidCorrelationDependency, idx, err)
		}
		if candidate.ScopeID != observation.ScopeID {
			continue
		}
		for _, signal := range candidate.Signals {
			if signal.ID == observation.Signal.ID {
				linkedOwners = append(linkedOwners, candidate)
				break
			}
		}
		if candidate.Status == StatusResolved || observation.Signal.ObservedAt.Before(candidate.StartedAt) {
			continue
		}
		if !sameResourceSet(candidate.AffectedResources, observation.AffectedResources) {
			continue
		}
		if !incidentHasSignalClass(candidate, observation.Signal.Kind, observation.Signal.Source) {
			continue
		}
		exactMatches = append(exactMatches, candidate)
	}

	if len(linkedOwners) > 1 {
		return blockedCorrelation(fingerprint, CorrelationBlockerSignalIDConflict), nil
	}
	if len(linkedOwners) == 1 {
		return matchedCorrelation(CorrelationAlreadyLinked, fingerprint, linkedOwners[0]), nil
	}
	if len(exactMatches) > 1 {
		return blockedCorrelation(fingerprint, CorrelationBlockerAmbiguousExactMatch), nil
	}
	if len(exactMatches) == 1 {
		return matchedCorrelation(CorrelationMatch, fingerprint, exactMatches[0]), nil
	}
	return CorrelationDecision{
		Contract:                  CorrelationContractV1,
		State:                     CorrelationNewCandidate,
		Fingerprint:               fingerprint,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}, nil
}

func validateCorrelationObservation(observation CorrelationObservation) error {
	if err := validateRef("correlation.scope_id", observation.ScopeID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorrelationObservation, err)
	}
	if err := validateResources(observation.AffectedResources); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorrelationObservation, err)
	}
	signal := observation.Signal
	if err := validateRef("correlation.signal.id", signal.ID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorrelationObservation, err)
	}
	if err := validateRef("correlation.signal.kind", signal.Kind); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorrelationObservation, err)
	}
	if err := validateRef("correlation.signal.source", signal.Source); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorrelationObservation, err)
	}
	if signal.ObservedAt.IsZero() {
		return fmt.Errorf("%w: signal observed_at is required", ErrInvalidCorrelationObservation)
	}
	if err := validateText("correlation.signal.summary", signal.Summary, maxSummaryLength, true); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorrelationObservation, err)
	}
	if len(signal.Evidence) > maxEvidenceRefs {
		return fmt.Errorf("%w: too many signal evidence references", ErrInvalidCorrelationObservation)
	}
	if err := validateEvidenceSet(signal.Evidence); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorrelationObservation, err)
	}
	return nil
}

func correlationFingerprint(observation CorrelationObservation) string {
	resources := canonicalResourceKeys(observation.AffectedResources)
	canonical := strings.Join([]string{
		CorrelationContractV1,
		observation.ScopeID,
		observation.Signal.Kind,
		observation.Signal.Source,
		strings.Join(resources, "\n"),
	}, "\x00")
	sum := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func canonicalResourceKeys(resources []ResourceRef) []string {
	keys := make([]string, 0, len(resources))
	for _, resource := range resources {
		keys = append(keys, resource.Kind+"\x00"+resource.ID+"\x00"+resource.ScopeID)
	}
	sort.Strings(keys)
	return keys
}

func sameResourceSet(left, right []ResourceRef) bool {
	if len(left) != len(right) {
		return false
	}
	leftKeys := canonicalResourceKeys(left)
	rightKeys := canonicalResourceKeys(right)
	for idx := range leftKeys {
		if leftKeys[idx] != rightKeys[idx] {
			return false
		}
	}
	return true
}

func incidentHasSignalClass(incident Incident, kind, source string) bool {
	for _, signal := range incident.Signals {
		if signal.Kind == kind && signal.Source == source {
			return true
		}
	}
	return false
}

func matchedCorrelation(state CorrelationState, fingerprint string, incident Incident) CorrelationDecision {
	return CorrelationDecision{
		Contract:                  CorrelationContractV1,
		State:                     state,
		Fingerprint:               fingerprint,
		IncidentID:                incident.ObjectID,
		IncidentResourceVersion:   incident.ResourceVersion,
		IncidentGeneration:        incident.Generation,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}
}

func blockedCorrelation(fingerprint, blocker string) CorrelationDecision {
	return CorrelationDecision{
		Contract:                  CorrelationContractV1,
		State:                     CorrelationBlocked,
		Fingerprint:               fingerprint,
		Blockers:                  []string{blocker},
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}
}
