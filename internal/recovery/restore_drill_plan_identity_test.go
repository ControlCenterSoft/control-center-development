package recovery

import (
	"testing"
	"time"
)

func TestBuildRestoreDrillPlanIDBindsRecoveryInputs(t *testing.T) {
	registry := registeredRestoreDrillRegistry(t)
	base := plannedRestoreDrill()
	evaluatedAt := base.UpdatedAt.Add(time.Minute)

	baseline, err := BuildRestoreDrillPlan(registry, base, evaluatedAt)
	if err != nil {
		t.Fatalf("BuildRestoreDrillPlan(baseline) error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*RestoreMetadata)
	}{
		{
			name: "recovery point",
			mutate: func(value *RestoreMetadata) {
				value.RecoveryPointID = "rp-20260908-002"
			},
		},
		{
			name: "backup set",
			mutate: func(value *RestoreMetadata) {
				value.BackupIDs = append(value.BackupIDs, "backup-002")
			},
		},
		{
			name: "restore target",
			mutate: func(value *RestoreMetadata) {
				value.Target.ObjectID = "database-restore-target-002"
			},
		},
		{
			name: "resource version",
			mutate: func(value *RestoreMetadata) {
				value.ResourceVersion = "rv:restore-drill-001:2"
			},
		},
		{
			name: "generation",
			mutate: func(value *RestoreMetadata) {
				value.Generation++
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneRestoreMetadata(base)
			test.mutate(&candidate)

			plan, err := BuildRestoreDrillPlan(registry, candidate, evaluatedAt)
			if err != nil {
				t.Fatalf("BuildRestoreDrillPlan(%s) error = %v", test.name, err)
			}
			if plan.PlanID == baseline.PlanID {
				t.Fatalf("PlanID did not change after %s changed: %s", test.name, plan.PlanID)
			}
		})
	}
}
