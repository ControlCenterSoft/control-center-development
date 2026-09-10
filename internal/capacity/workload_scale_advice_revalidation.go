package capacity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	WorkloadScaleAdviceSnapshotSchemaV1     = "capacity.workload-scale-advice-snapshot/v1"
	WorkloadScaleAdviceRevalidationSchemaV1 = "capacity.workload-scale-advice-revalidation/v1"
)

type WorkloadScaleAdviceRevalidationStatus string

const (
	WorkloadScaleAdviceEvidenceCurrent WorkloadScaleAdviceRevalidationStatus = "current"
	WorkloadScaleAdviceEvidenceStale   WorkloadScaleAdviceRevalidationStatus = "stale"
)

type WorkloadScaleAdviceSnapshot struct {
	SchemaVersion      string                        `json:"schema_version"`
	SnapshotID         string                        `json:"snapshot_id"`
	CurveID            string                        `json:"curve_id"`
	EfficiencyReportID string                        `json:"efficiency_report_id"`
	OptionsID          string                        `json:"options_id"`
	BottleneckReportID string                        `json:"bottleneck_report_id"`
	AdviceFingerprint  string                        `json:"advice_fingerprint"`
	Advice             WorkloadScaleBottleneckAdvice `json:"advice"`
	AdvisoryOnly       bool                          `json:"advisory_only"`
	ProductionMutation bool                          `json:"production_mutation"`
}

type WorkloadScaleAdviceRevalidation struct {
	SchemaVersion             string                                `json:"schema_version"`
	RevalidationID            string                                `json:"revalidation_id"`
	SnapshotID                string                                `json:"snapshot_id"`
	CurrentSnapshotID         string                                `json:"current_snapshot_id"`
	AdviceID                  string                                `json:"advice_id"`
	CurrentAdviceID           string                                `json:"current_advice_id"`
	CurveID                   string                                `json:"curve_id"`
	CurrentCurveID            string                                `json:"current_curve_id"`
	EfficiencyReportID        string                                `json:"efficiency_report_id"`
	CurrentEfficiencyReportID string                                `json:"current_efficiency_report_id"`
	OptionsID                 string                                `json:"options_id"`
	CurrentOptionsID          string                                `json:"current_options_id"`
	BottleneckReportID        string                                `json:"bottleneck_report_id"`
	CurrentBottleneckReportID string                                `json:"current_bottleneck_report_id"`
	Status                    WorkloadScaleAdviceRevalidationStatus `json:"status"`
	Reason                    string                                `json:"reason"`
	RecommendationReusable    bool                                  `json:"recommendation_reusable"`
	RecommendedAction         string                                `json:"recommended_action"`
	AdvisoryOnly              bool                                  `json:"advisory_only"`
	ProductionMutation        bool                                  `json:"production_mutation"`
}

// CaptureWorkloadScaleAdviceSnapshot seals one advisory decision to the exact
// curve, efficiency, option and bottleneck evidence that produced it. The
// snapshot is evidence only and never authorizes a placement or scaling change.
func CaptureWorkloadScaleAdviceSnapshot(
	curve WorkloadCurve,
	efficiency WorkloadCurveEfficiencyReport,
	requests []WorkloadScaleScenarioRequest,
	bottleneckRequest BottleneckRequest,
	nodes []NodeProjection,
) (WorkloadScaleAdviceSnapshot, error) {
	options, err := EvaluateWorkloadScaleOptions(curve, efficiency, requests)
	if err != nil {
		return WorkloadScaleAdviceSnapshot{}, err
	}
	bottleneck, err := BuildBottleneckReport(bottleneckRequest, nodes)
	if err != nil {
		return WorkloadScaleAdviceSnapshot{}, err
	}
	advice, err := BuildWorkloadScaleBottleneckAdvice(curve, efficiency, requests, bottleneckRequest, nodes)
	if err != nil {
		return WorkloadScaleAdviceSnapshot{}, err
	}
	return newWorkloadScaleAdviceSnapshot(curve, efficiency, options, bottleneck, advice)
}

// RevalidateWorkloadScaleAdviceSnapshot rebuilds planner evidence from the
// current bounded inputs and invalidates reuse on any exact evidence drift. A
// current result remains advisory; callers must not interpret it as execution
// authority for a placement or scaling mutation.
func RevalidateWorkloadScaleAdviceSnapshot(
	saved WorkloadScaleAdviceSnapshot,
	curve WorkloadCurve,
	efficiency WorkloadCurveEfficiencyReport,
	requests []WorkloadScaleScenarioRequest,
	bottleneckRequest BottleneckRequest,
	nodes []NodeProjection,
) (WorkloadScaleAdviceRevalidation, error) {
	if err := validateWorkloadScaleAdviceSnapshot(saved); err != nil {
		return WorkloadScaleAdviceRevalidation{}, err
	}
	current, err := CaptureWorkloadScaleAdviceSnapshot(curve, efficiency, requests, bottleneckRequest, nodes)
	if err != nil {
		return WorkloadScaleAdviceRevalidation{}, err
	}

	result := WorkloadScaleAdviceRevalidation{
		SchemaVersion:             WorkloadScaleAdviceRevalidationSchemaV1,
		SnapshotID:                saved.SnapshotID,
		CurrentSnapshotID:         current.SnapshotID,
		AdviceID:                  saved.Advice.AdviceID,
		CurrentAdviceID:           current.Advice.AdviceID,
		CurveID:                   saved.CurveID,
		CurrentCurveID:            current.CurveID,
		EfficiencyReportID:        saved.EfficiencyReportID,
		CurrentEfficiencyReportID: current.EfficiencyReportID,
		OptionsID:                 saved.OptionsID,
		CurrentOptionsID:          current.OptionsID,
		BottleneckReportID:        saved.BottleneckReportID,
		CurrentBottleneckReportID: current.BottleneckReportID,
		Status:                    WorkloadScaleAdviceEvidenceStale,
		Reason:                    "advice-decision-drift",
		RecommendationReusable:    false,
		RecommendedAction:         "rebuild-advice-from-current-evidence",
		AdvisoryOnly:              true,
		ProductionMutation:        false,
	}

	switch {
	case saved.CurveID != current.CurveID:
		result.Reason = "curve-evidence-drift"
	case saved.EfficiencyReportID != current.EfficiencyReportID:
		result.Reason = "efficiency-evidence-drift"
	case saved.OptionsID != current.OptionsID:
		result.Reason = "scale-options-evidence-drift"
	case saved.BottleneckReportID != current.BottleneckReportID:
		result.Reason = "bottleneck-evidence-drift"
	case saved.AdviceFingerprint != current.AdviceFingerprint || saved.Advice.AdviceID != current.Advice.AdviceID:
		result.Reason = "advice-decision-drift"
	case saved.SnapshotID != current.SnapshotID:
		result.Reason = "snapshot-identity-drift"
	default:
		result.Status = WorkloadScaleAdviceEvidenceCurrent
		result.Reason = "exact-evidence-current"
		result.RecommendedAction = "none"
		result.RecommendationReusable = reusableScaleRecommendation(saved.Advice)
	}

	result.RevalidationID = workloadScaleAdviceRevalidationID(result)
	return result, nil
}

func newWorkloadScaleAdviceSnapshot(
	curve WorkloadCurve,
	efficiency WorkloadCurveEfficiencyReport,
	options WorkloadScaleOptions,
	bottleneck BottleneckReport,
	advice WorkloadScaleBottleneckAdvice,
) (WorkloadScaleAdviceSnapshot, error) {
	if curve.SchemaVersion != WorkloadCurveSchemaV1 || !curve.AdvisoryOnly || curve.ProductionMutation ||
		efficiency.SchemaVersion != WorkloadCurveEfficiencySchemaV1 || !efficiency.AdvisoryOnly || efficiency.ProductionMutation ||
		options.SchemaVersion != WorkloadScaleOptionsSchemaV1 || !options.AdvisoryOnly || options.ProductionMutation ||
		bottleneck.SchemaVersion != BottleneckReportSchemaV1 || !bottleneck.AdvisoryOnly || bottleneck.ProductionMutation ||
		advice.SchemaVersion != WorkloadScaleBottleneckAdviceSchemaV1 || !advice.AdvisoryOnly || advice.ProductionMutation {
		return WorkloadScaleAdviceSnapshot{}, fmt.Errorf("%w: unsafe planner evidence", ErrInvalidRecommendation)
	}
	if efficiency.CurveID != curve.CurveID || options.CurveID != curve.CurveID ||
		options.EfficiencyReportID != efficiency.ReportID || advice.OptionsID != options.OptionsID ||
		advice.BottleneckReportID != bottleneck.ReportID {
		return WorkloadScaleAdviceSnapshot{}, fmt.Errorf("%w: planner evidence identity mismatch", ErrInvalidRecommendation)
	}

	fingerprint, err := workloadScaleAdviceFingerprint(advice)
	if err != nil {
		return WorkloadScaleAdviceSnapshot{}, err
	}
	snapshot := WorkloadScaleAdviceSnapshot{
		SchemaVersion:      WorkloadScaleAdviceSnapshotSchemaV1,
		CurveID:            curve.CurveID,
		EfficiencyReportID: efficiency.ReportID,
		OptionsID:          options.OptionsID,
		BottleneckReportID: bottleneck.ReportID,
		AdviceFingerprint:  fingerprint,
		Advice:             advice,
		AdvisoryOnly:       true,
		ProductionMutation: false,
	}
	snapshot.SnapshotID = workloadScaleAdviceSnapshotID(snapshot)
	return snapshot, nil
}

func validateWorkloadScaleAdviceSnapshot(snapshot WorkloadScaleAdviceSnapshot) error {
	if snapshot.SchemaVersion != WorkloadScaleAdviceSnapshotSchemaV1 || !snapshot.AdvisoryOnly || snapshot.ProductionMutation ||
		snapshot.Advice.SchemaVersion != WorkloadScaleBottleneckAdviceSchemaV1 || !snapshot.Advice.AdvisoryOnly || snapshot.Advice.ProductionMutation {
		return fmt.Errorf("%w: unsafe saved scale advice snapshot", ErrInvalidRecommendation)
	}
	if strings.TrimSpace(snapshot.CurveID) == "" || strings.TrimSpace(snapshot.EfficiencyReportID) == "" ||
		strings.TrimSpace(snapshot.OptionsID) == "" || strings.TrimSpace(snapshot.BottleneckReportID) == "" ||
		snapshot.Advice.OptionsID != snapshot.OptionsID || snapshot.Advice.BottleneckReportID != snapshot.BottleneckReportID {
		return fmt.Errorf("%w: saved scale advice identity mismatch", ErrInvalidRecommendation)
	}
	fingerprint, err := workloadScaleAdviceFingerprint(snapshot.Advice)
	if err != nil {
		return err
	}
	if fingerprint != snapshot.AdviceFingerprint || workloadScaleAdviceSnapshotID(snapshot) != snapshot.SnapshotID {
		return fmt.Errorf("%w: saved scale advice snapshot integrity mismatch", ErrInvalidRecommendation)
	}
	return nil
}

func workloadScaleAdviceFingerprint(advice WorkloadScaleBottleneckAdvice) (string, error) {
	encoded, err := json.Marshal(advice)
	if err != nil {
		return "", fmt.Errorf("%w: advice fingerprint: %v", ErrInvalidRecommendation, err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func workloadScaleAdviceSnapshotID(snapshot WorkloadScaleAdviceSnapshot) string {
	canonical := struct {
		CurveID            string `json:"curve_id"`
		EfficiencyReportID string `json:"efficiency_report_id"`
		OptionsID          string `json:"options_id"`
		BottleneckReportID string `json:"bottleneck_report_id"`
		AdviceFingerprint  string `json:"advice_fingerprint"`
	}{snapshot.CurveID, snapshot.EfficiencyReportID, snapshot.OptionsID, snapshot.BottleneckReportID, snapshot.AdviceFingerprint}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return "wsas-" + hex.EncodeToString(digest[:])[:24]
}

func workloadScaleAdviceRevalidationID(result WorkloadScaleAdviceRevalidation) string {
	canonical := struct {
		SnapshotID        string                                `json:"snapshot_id"`
		CurrentSnapshotID string                                `json:"current_snapshot_id"`
		Status            WorkloadScaleAdviceRevalidationStatus `json:"status"`
		Reason            string                                `json:"reason"`
	}{result.SnapshotID, result.CurrentSnapshotID, result.Status, result.Reason}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return "wsar-" + hex.EncodeToString(digest[:])[:24]
}

func reusableScaleRecommendation(advice WorkloadScaleBottleneckAdvice) bool {
	if advice.RecommendedScenarioID == "" || advice.RecommendedResourceFactor == nil {
		return false
	}
	return advice.Status == WorkloadScaleAdviceRecommendationAvailable || advice.Status == WorkloadScaleAdviceRecommendationCapped
}
