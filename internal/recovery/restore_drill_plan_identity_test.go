package recovery

import (
	"testing"
	"time"
)

func TestBuildRestoreDrillPlanIDBindsRecoveryInputs(t *testing.T) {
	registry := registeredRestoreDrillRegistry(t)
	alternateAdapter := restoreDrillAdapter()
	alternateAdapter.Version = "2.55.0"
	if err := registry.Register(alternateAdapter); err != nil {
		t.Fatalf("Register(alternate adapter) error = %v", err)
	}

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
			name: "restore identity",
			mutate: func(value *RestoreMetadata) {
				value.ObjectID = "restore-drill-002"
			},
		},
		{
			name: "scope boundary",
			mutate: func(value *RestoreMetadata) {
				value.ScopeID = "scope-secondary"
				value.Target.ScopeID = "scope-secondary"
			},
		},
		{
			name: "owner scope",
			mutate: func(value *RestoreMetadata) {
				value.OwnerScope = "global"
			},
		},
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
			name: "provider identity",
			mutate: func(value *RestoreMetadata) {
				value.Provider.ProviderID = "pgbackrest-secondary"
			},
		},
		{
			name: "backup repository",
			mutate: func(value *RestoreMetadata) {
				value.Provider.RepositoryID = "backup-repository-b"
			},
		},
		{
			name: "adapter version",
			mutate: func(value *RestoreMetadata) {
				value.Provider.Version = "2.55.0"
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
