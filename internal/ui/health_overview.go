package ui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corehealth "control-center/internal/health"
)

const HealthOverviewContractVersion = "ui.health-overview/v1"

const (
	maxHealthSignals       = 1000
	maxEvidenceRefsPerItem = 32
	maxHealthTokenLength    = 256
)

var ErrInvalidHealthOverview = errors.New("invalid health overview")

type HealthOverviewDataState string

const (
	HealthOverviewUnavailable HealthOverviewDataState = "unavailable"
	HealthOverviewCurrent     HealthOverviewDataState = "current"
)

type HealthFreshness string

const (
	HealthFreshnessCurrent HealthFreshness = "current"
	HealthFreshnessStale   HealthFreshness = "stale"
	HealthFreshnessExpired HealthFreshness = "expired"
)

// HealthSignal is a bounded, already-authorized source signal. Message/body
// fields are deliberately absent so the UI projection cannot accidentally
// surface provider errors, credentials or other unreviewed diagnostic text.
type HealthSignal struct {
	ID           string
	ResourceKind string
	ResourceID   string
	CheckName    string
	State        corehealth.State
	ObservedAt   time.Time
	RunbookRef   string
	EvidenceRefs []string
}

type HealthOverviewInput struct {
	Signals      []HealthSignal
	Loaded       bool
	Now          time.Time
	StaleAfter   time.Duration
	ExpiredAfter time.Duration
}

type HealthSignalView struct {
	ID             string           `json:"id"`
	ResourceKind   string           `json:"resource_kind"`
	ResourceID     string           `json:"resource_id"`
	CheckName      string           `json:"check_name"`
	SourceState    corehealth.State `json:"source_state"`
	EffectiveState corehealth.State `json:"effective_state"`
	Freshness      HealthFreshness  `json:"freshness"`
	ObservedAt     time.Time        `json:"observed_at"`
	RunbookRef     string           `json:"runbook_ref,omitempty"`
	EvidenceRefs   []string         `json:"evidence_refs"`
}

type AffectedResourceView struct {
	ResourceKind string           `json:"resource_kind"`
	ResourceID   string           `json:"resource_id"`
	State        corehealth.State `json:"state"`
	Freshness    HealthFreshness  `json:"freshness"`
	SignalCount  int              `json:"signal_count"`
}

type HealthOverviewView struct {
	ContractVersion    string                 `json:"contract_version"`
	DataState          HealthOverviewDataState `json:"data_state"`
	GeneratedAt        *time.Time             `json:"generated_at,omitempty"`
	OverallState       corehealth.State       `json:"overall_state"`
	SignalCount        int                    `json:"signal_count"`
	AffectedResources  []AffectedResourceView `json:"affected_resources"`
	Signals            []HealthSignalView      `json:"signals"`
	MutationAuthorized bool                    `json:"mutation_authorized"`
}

// BuildHealthOverview projects health evidence into a deterministic read-only
// operator view. Fresh or stale data is never silently promoted to Healthy:
// a stale healthy signal becomes Degraded and an expired healthy signal becomes
// Unknown. The projection never authorizes acknowledgement, remediation or any
// infrastructure mutation.
func BuildHealthOverview(input HealthOverviewInput) (HealthOverviewView, error) {
	view := HealthOverviewView{
		ContractVersion:    HealthOverviewContractVersion,
		DataState:          HealthOverviewUnavailable,
		OverallState:       corehealth.StateUnknown,
		AffectedResources:  []AffectedResourceView{},
		Signals:            []HealthSignalView{},
		MutationAuthorized: false,
	}
	if !input.Loaded {
		return view, nil
	}
	if input.Now.IsZero() {
		return HealthOverviewView{}, invalidHealthOverview("current time is required")
	}
	if input.StaleAfter <= 0 || input.ExpiredAfter <= input.StaleAfter {
		return HealthOverviewView{}, invalidHealthOverview("freshness budgets are invalid")
	}
	if len(input.Signals) > maxHealthSignals {
		return HealthOverviewView{}, invalidHealthOverview("signal count exceeds %d", maxHealthSignals)
	}

	now := input.Now.UTC()
	view.DataState = HealthOverviewCurrent
	view.GeneratedAt = &now

	seenSignals := make(map[string]struct{}, len(input.Signals))
	resources := make(map[string]AffectedResourceView)
	for _, source := range input.Signals {
		projected, err := projectHealthSignal(source, now, input.StaleAfter, input.ExpiredAfter)
		if err != nil {
			return HealthOverviewView{}, err
		}
		if _, exists := seenSignals[projected.ID]; exists {
			return HealthOverviewView{}, invalidHealthOverview("duplicate signal %q", projected.ID)
		}
		seenSignals[projected.ID] = struct{}{}
		view.Signals = append(view.Signals, projected)

		key := projected.ResourceKind + "\x00" + projected.ResourceID
		resource, exists := resources[key]
		if !exists {
			resource = AffectedResourceView{
				ResourceKind: projected.ResourceKind,
				ResourceID:   projected.ResourceID,
				State:        projected.EffectiveState,
				Freshness:    projected.Freshness,
			}
		}
		resource.SignalCount++
		if healthSeverity(projected.EffectiveState) > healthSeverity(resource.State) {
			resource.State = projected.EffectiveState
		}
		if freshnessSeverity(projected.Freshness) > freshnessSeverity(resource.Freshness) {
			resource.Freshness = projected.Freshness
		}
		resources[key] = resource
	}

	if len(view.Signals) > 0 {
		view.OverallState = corehealth.StateHealthy
		for _, signal := range view.Signals {
			if healthSeverity(signal.EffectiveState) > healthSeverity(view.OverallState) {
				view.OverallState = signal.EffectiveState
			}
		}
	}
	view.SignalCount = len(view.Signals)

	for _, resource := range resources {
		view.AffectedResources = append(view.AffectedResources, resource)
	}
	sort.Slice(view.AffectedResources, func(i, j int) bool {
		left, right := view.AffectedResources[i], view.AffectedResources[j]
		if healthSeverity(left.State) != healthSeverity(right.State) {
			return healthSeverity(left.State) > healthSeverity(right.State)
		}
		if left.ResourceKind != right.ResourceKind {
			return left.ResourceKind < right.ResourceKind
		}
		return left.ResourceID < right.ResourceID
	})
	sort.Slice(view.Signals, func(i, j int) bool {
		left, right := view.Signals[i], view.Signals[j]
		if healthSeverity(left.EffectiveState) != healthSeverity(right.EffectiveState) {
			return healthSeverity(left.EffectiveState) > healthSeverity(right.EffectiveState)
		}
		if !left.ObservedAt.Equal(right.ObservedAt) {
			return left.ObservedAt.After(right.ObservedAt)
		}
		return left.ID < right.ID
	})
	return view, nil
}

func projectHealthSignal(source HealthSignal, now time.Time, staleAfter, expiredAfter time.Duration) (HealthSignalView, error) {
	if err := validateHealthToken("signal id", source.ID, 128, true); err != nil {
		return HealthSignalView{}, err
	}
	if err := validateHealthToken("resource kind", source.ResourceKind, 64, true); err != nil {
		return HealthSignalView{}, err
	}
	if err := validateHealthToken("resource id", source.ResourceID, 128, true); err != nil {
		return HealthSignalView{}, err
	}
	if err := validateHealthToken("check name", source.CheckName, 128, true); err != nil {
		return HealthSignalView{}, err
	}
	if !validHealthState(source.State) {
		return HealthSignalView{}, invalidHealthOverview("signal %q has invalid health state", source.ID)
	}
	if source.ObservedAt.IsZero() || source.ObservedAt.After(now) {
		return HealthSignalView{}, invalidHealthOverview("signal %q has invalid observation time", source.ID)
	}
	if source.RunbookRef != "" {
		if err := validateHealthToken("runbook reference", source.RunbookRef, maxHealthTokenLength, false); err != nil {
			return HealthSignalView{}, err
		}
	}
	if len(source.EvidenceRefs) > maxEvidenceRefsPerItem {
		return HealthSignalView{}, invalidHealthOverview("signal %q has too many evidence references", source.ID)
	}

	evidence := make([]string, 0, len(source.EvidenceRefs))
	seenEvidence := make(map[string]struct{}, len(source.EvidenceRefs))
	for _, reference := range source.EvidenceRefs {
		if err := validateHealthToken("evidence reference", reference, maxHealthTokenLength, true); err != nil {
			return HealthSignalView{}, err
		}
		if _, exists := seenEvidence[reference]; exists {
			continue
		}
		seenEvidence[reference] = struct{}{}
		evidence = append(evidence, reference)
	}
	sort.Strings(evidence)

	observedAt := source.ObservedAt.UTC()
	age := now.Sub(observedAt)
	freshness := HealthFreshnessCurrent
	switch {
	case age > expiredAfter:
		freshness = HealthFreshnessExpired
	case age > staleAfter:
		freshness = HealthFreshnessStale
	}

	effective := source.State
	if source.State == corehealth.StateHealthy {
		switch freshness {
		case HealthFreshnessStale:
			effective = corehealth.StateDegraded
		case HealthFreshnessExpired:
			effective = corehealth.StateUnknown
		}
	}
	return HealthSignalView{
		ID:             source.ID,
		ResourceKind:   source.ResourceKind,
		ResourceID:     source.ResourceID,
		CheckName:      source.CheckName,
		SourceState:    source.State,
		EffectiveState: effective,
		Freshness:      freshness,
		ObservedAt:     observedAt,
		RunbookRef:     source.RunbookRef,
		EvidenceRefs:   evidence,
	}, nil
}

func validateHealthToken(name, value string, max int, required bool) error {
	if value == "" && !required {
		return nil
	}
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > max {
		return invalidHealthOverview("%s is not canonical or exceeds the bound", name)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return invalidHealthOverview("%s contains a control character", name)
		}
	}
	return nil
}

func validHealthState(state corehealth.State) bool {
	switch state {
	case corehealth.StateHealthy, corehealth.StateUnknown, corehealth.StateDegraded, corehealth.StateUnhealthy:
		return true
	default:
		return false
	}
}

func healthSeverity(state corehealth.State) int {
	switch state {
	case corehealth.StateHealthy:
		return 0
	case corehealth.StateUnknown:
		return 1
	case corehealth.StateDegraded:
		return 2
	case corehealth.StateUnhealthy:
		return 3
	default:
		return 4
	}
}

func freshnessSeverity(freshness HealthFreshness) int {
	switch freshness {
	case HealthFreshnessCurrent:
		return 0
	case HealthFreshnessStale:
		return 1
	case HealthFreshnessExpired:
		return 2
	default:
		return 3
	}
}

func invalidHealthOverview(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidHealthOverview, fmt.Sprintf(format, values...))
}
