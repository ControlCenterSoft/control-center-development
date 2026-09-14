package nodelifecycle

import "testing"

func TestOperationPlanIDBindsSafetyCriticalPlacementEvidence(t *testing.T) {
	current := lifecycleInState(StateMaintenance)
	base := OperationPlanRequest{
		Kind:              OperationReplace,
		ReplacementNodeID: "node-2",
		Placements: []WorkloadPlacement{
			{
				WorkloadID:             "database-1",
				Kind:                   WorkloadStateful,
				MigrationAdapter:       "postgres-replica-switchover",
				HealthyReplicasOutside: 1,
			},
			{WorkloadID: "web-1", Kind: WorkloadStateless},
		},
	}

	build := func(t *testing.T, request OperationPlanRequest) OperationPlan {
		t.Helper()
		plan, err := BuildOperationPlan(current, request)
		if err != nil {
			t.Fatalf("BuildOperationPlan() error = %v", err)
		}
		return plan
	}

	baseline := build(t, base)

	adapterChanged := base
	adapterChanged.Placements = append([]WorkloadPlacement(nil), base.Placements...)
	adapterChanged.Placements[0].MigrationAdapter = "postgres-logical-switchover"
	if got := build(t, adapterChanged).PlanID; got == baseline.PlanID {
		t.Fatalf("PlanID did not change when migration adapter changed: %q", got)
	}

	replicaEvidenceChanged := base
	replicaEvidenceChanged.Placements = append([]WorkloadPlacement(nil), base.Placements...)
	replicaEvidenceChanged.Placements[0].HealthyReplicasOutside = 2
	if got := build(t, replicaEvidenceChanged).PlanID; got == baseline.PlanID {
		t.Fatalf("PlanID did not change when healthy replica evidence changed: %q", got)
	}
}
