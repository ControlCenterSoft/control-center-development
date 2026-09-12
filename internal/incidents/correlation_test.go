package incidents

import (
	"errors"
	"testing"
	"time"
)

func TestEvaluateCorrelationMatchesExactOpenIncidentDeterministically(t *testing.T) {
	candidate := validIncident(StatusAcknowledged)
	candidate.ObjectID = "incident:exact"
	candidate.ResourceVersion = "rv-exact"
	candidate.Generation = 7
	candidate.AffectedResources = append(candidate.AffectedResources,
		ResourceRef{Kind: "service", ID: "dns", ScopeID: "site:primary"},
	)

	observation := correlationObservation(candidate)
	observation.Signal.ID = "signal-new"
	observation.Signal.ObservedAt = candidate.LastObservedAt.Add(time.Minute)
	observation.AffectedResources[0], observation.AffectedResources[1] = observation.AffectedResources[1], observation.AffectedResources[0]

	decision, err := EvaluateCorrelation(observation, []Incident{candidate})
	if err != nil {
		t.Fatalf("EvaluateCorrelation() error = %v", err)
	}
	if decision.State != CorrelationMatch {
		t.Fatalf("state = %q, want %q", decision.State, CorrelationMatch)
	}
	if decision.IncidentID != candidate.ObjectID || decision.IncidentResourceVersion != candidate.ResourceVersion || decision.IncidentGeneration != candidate.Generation {
		t.Fatalf("decision not bound to exact incident revision: %#v", decision)
	}
	if decision.Fingerprint == "" || decision.ExecutionAuthorized || decision.ProductionMutationAllowed {
		t.Fatalf("unsafe or incomplete decision: %#v", decision)
	}
}

func TestEvaluateCorrelationFingerprintIgnoresResourceOrder(t *testing.T) {
	candidate := validIncident(StatusOpen)
	candidate.AffectedResources = append(candidate.AffectedResources,
		ResourceRef{Kind: "service", ID: "dns", ScopeID: "site:primary"},
	)
	first := correlationObservation(candidate)
	first.Signal.ID = "signal-a"
	first.Signal.ObservedAt = candidate.LastObservedAt.Add(time.Minute)
	second := first
	second.Signal.ID = "signal-b"
	second.Signal.ObservedAt = first.Signal.ObservedAt.Add(time.Minute)
	second.AffectedResources = append([]ResourceRef(nil), first.AffectedResources...)
	second.AffectedResources[0], second.AffectedResources[1] = second.AffectedResources[1], second.AffectedResources[0]

	firstDecision, err := EvaluateCorrelation(first, nil)
	if err != nil {
		t.Fatalf("first EvaluateCorrelation() error = %v", err)
	}
	secondDecision, err := EvaluateCorrelation(second, nil)
	if err != nil {
		t.Fatalf("second EvaluateCorrelation() error = %v", err)
	}
	if firstDecision.Fingerprint != secondDecision.Fingerprint {
		t.Fatalf("fingerprints differ by resource order: %q != %q", firstDecision.Fingerprint, secondDecision.Fingerprint)
	}
}

func TestEvaluateCorrelationBlocksMultipleExactOpenIncidents(t *testing.T) {
	first := validIncident(StatusOpen)
	first.ObjectID = "incident:first"
	first.ResourceVersion = "rv-first"
	second := first
	second.ObjectID = "incident:second"
	second.ResourceVersion = "rv-second"

	observation := correlationObservation(first)
	observation.Signal.ID = "signal-new"
	observation.Signal.ObservedAt = first.LastObservedAt.Add(time.Minute)

	decision, err := EvaluateCorrelation(observation, []Incident{first, second})
	if err != nil {
		t.Fatalf("EvaluateCorrelation() error = %v", err)
	}
	if decision.State != CorrelationBlocked || len(decision.Blockers) != 1 || decision.Blockers[0] != CorrelationBlockerAmbiguousExactMatch {
		t.Fatalf("decision = %#v", decision)
	}
	if decision.IncidentID != "" {
		t.Fatalf("ambiguous decision leaked an incident choice: %#v", decision)
	}
}

func TestEvaluateCorrelationReturnsAlreadyLinkedForKnownSignal(t *testing.T) {
	candidate := validIncident(StatusResolved)
	observation := correlationObservation(candidate)

	decision, err := EvaluateCorrelation(observation, []Incident{candidate})
	if err != nil {
		t.Fatalf("EvaluateCorrelation() error = %v", err)
	}
	if decision.State != CorrelationAlreadyLinked || decision.IncidentID != candidate.ObjectID {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestEvaluateCorrelationBlocksSignalIDOwnedByMultipleIncidents(t *testing.T) {
	first := validIncident(StatusAcknowledged)
	first.ObjectID = "incident:first"
	first.ResourceVersion = "rv-first"
	second := validIncident(StatusAcknowledged)
	second.ObjectID = "incident:second"
	second.ResourceVersion = "rv-second"

	observation := correlationObservation(first)
	decision, err := EvaluateCorrelation(observation, []Incident{first, second})
	if err != nil {
		t.Fatalf("EvaluateCorrelation() error = %v", err)
	}
	if decision.State != CorrelationBlocked || len(decision.Blockers) != 1 || decision.Blockers[0] != CorrelationBlockerSignalIDConflict {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestEvaluateCorrelationResolvedIncidentDoesNotAbsorbNewSignal(t *testing.T) {
	candidate := validIncident(StatusResolved)
	observation := correlationObservation(candidate)
	observation.Signal.ID = "signal-new"
	observation.Signal.ObservedAt = candidate.UpdatedAt.Add(time.Minute)

	decision, err := EvaluateCorrelation(observation, []Incident{candidate})
	if err != nil {
		t.Fatalf("EvaluateCorrelation() error = %v", err)
	}
	if decision.State != CorrelationNewCandidate {
		t.Fatalf("state = %q, want %q", decision.State, CorrelationNewCandidate)
	}
}

func TestEvaluateCorrelationDifferentScopeOrResourcesDoesNotMatch(t *testing.T) {
	candidate := validIncident(StatusOpen)
	observation := correlationObservation(candidate)
	observation.Signal.ID = "signal-new"
	observation.Signal.ObservedAt = candidate.LastObservedAt.Add(time.Minute)
	observation.AffectedResources = []ResourceRef{{Kind: "node", ID: "node-2", ScopeID: "site:primary"}}

	decision, err := EvaluateCorrelation(observation, []Incident{candidate})
	if err != nil {
		t.Fatalf("EvaluateCorrelation() error = %v", err)
	}
	if decision.State != CorrelationNewCandidate {
		t.Fatalf("state = %q, want %q", decision.State, CorrelationNewCandidate)
	}
}

func TestEvaluateCorrelationFailsClosedOnInvalidStoredIncident(t *testing.T) {
	candidate := validIncident(StatusOpen)
	candidate.ResourceVersion = ""
	observation := correlationObservation(validIncident(StatusOpen))
	observation.Signal.ID = "signal-new"

	_, err := EvaluateCorrelation(observation, []Incident{candidate})
	if !errors.Is(err, ErrInvalidCorrelationDependency) {
		t.Fatalf("EvaluateCorrelation() error = %v, want ErrInvalidCorrelationDependency", err)
	}
}

func TestEvaluateCorrelationRejectsInvalidObservation(t *testing.T) {
	observation := correlationObservation(validIncident(StatusOpen))
	observation.Signal.ObservedAt = time.Time{}

	_, err := EvaluateCorrelation(observation, nil)
	if !errors.Is(err, ErrInvalidCorrelationObservation) {
		t.Fatalf("EvaluateCorrelation() error = %v, want ErrInvalidCorrelationObservation", err)
	}
}

func TestEvaluateCorrelationRejectsUnboundedCandidateSet(t *testing.T) {
	observation := correlationObservation(validIncident(StatusOpen))
	candidates := make([]Incident, maxCorrelationInputs+1)

	_, err := EvaluateCorrelation(observation, candidates)
	if !errors.Is(err, ErrInvalidCorrelationDependency) {
		t.Fatalf("EvaluateCorrelation() error = %v, want ErrInvalidCorrelationDependency", err)
	}
}

func correlationObservation(incident Incident) CorrelationObservation {
	signal := incident.Signals[0]
	return CorrelationObservation{
		ScopeID:           incident.ScopeID,
		Signal:            signal,
		AffectedResources: append([]ResourceRef(nil), incident.AffectedResources...),
	}
}
