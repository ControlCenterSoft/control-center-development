package main

import (
	"net/http"

	automationapi "control-center/internal/automation/httpapi"
	identityapi "control-center/internal/identity/httpapi"
	"control-center/internal/identity/rbac"
	marketapi "control-center/internal/market/httpapi"
	nodesapi "control-center/internal/nodes/httpapi"
	pxeapi "control-center/internal/pxe/httpapi"
)

func newProductHandler(identity *identityapi.Server) http.Handler {
	mux := http.NewServeMux()
	guard := func(permission rbac.Permission, handler http.Handler) http.Handler {
		return identity.Authenticate(identity.Require(permission, rbac.GlobalScope())(handler))
	}

	mux.Handle("/api/v1/nodes/enrollment/plan", guard(rbac.PermissionNodeEnrollmentPlan, nodesapi.New()))
	mux.Handle("/api/v1/automation/plan", guard(rbac.PermissionAutomationPlan, automationapi.New()))
	mux.Handle("/api/v1/pxe/plan", guard(rbac.PermissionPXEPlan, pxeapi.New()))
	marketHandler := guard(rbac.PermissionMarketRead, marketapi.New())
	mux.Handle("/api/v1/market/manifests", marketHandler)
	mux.Handle("/api/v1/market/manifests/", marketHandler)
	return mux
}
