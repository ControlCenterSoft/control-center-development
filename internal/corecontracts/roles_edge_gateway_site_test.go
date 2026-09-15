package corecontracts

import (
	"errors"
	"testing"
)

func TestValidateRoleAssignmentRequiresSiteForEdgeGateway(t *testing.T) {
	topology := testTopology(t)
	assignment := testRoleAssignment("edge-gateway-1", RoleEdgeGateway)
	if err := ValidateRoleAssignment(assignment, topology); err != nil {
		t.Fatalf("ValidateRoleAssignment(valid edge gateway) error = %v", err)
	}

	withoutSite := assignment
	withoutSite.SiteID = ""
	if err := ValidateRoleAssignment(withoutSite, topology); !errors.Is(err, ErrInvalidRoleAssignment) {
		t.Fatalf("ValidateRoleAssignment(edge gateway without site) error = %v, want ErrInvalidRoleAssignment", err)
	}
}
