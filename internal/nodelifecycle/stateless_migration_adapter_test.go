package nodelifecycle

import (
	"errors"
	"testing"
)

func TestBuildOperationPlanRejectsMigrationAdapterForStatelessPlacement(t *testing.T) {
	current := lifecycleInState(StateReady)
	for _, adapter := range []string{"postgres-replica-switchover", " "} {
		t.Run(adapter, func(t *testing.T) {
			_, err := BuildOperationPlan(current, OperationPlanRequest{
				Kind: OperationDrain,
				Placements: []WorkloadPlacement{{
					WorkloadID:       "web-1",
					Kind:             WorkloadStateless,
					MigrationAdapter: adapter,
				}},
			})
			if !errors.Is(err, ErrInvalidOperationPlan) {
				t.Fatalf("error = %v, want ErrInvalidOperationPlan", err)
			}
		})
	}
}
