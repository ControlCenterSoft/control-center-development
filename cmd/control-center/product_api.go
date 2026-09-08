package main

import (
	"net/http"

	agentapi "control-center/internal/agent/httpapi"
	automationapi "control-center/internal/automation/httpapi"
	domainapi "control-center/internal/domain/httpapi"
	identityapi "control-center/internal/identity/httpapi"
	"control-center/internal/identity/rbac"
	inventoryapi "control-center/internal/inventory/httpapi"
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
	mux.Handle("/api/v1/domain/provider/resolve", guard(rbac.PermissionDomainProviderResolve, domainapi.ProviderHandler()))
	mux.Handle("/api/v1/domain/lifecycle/plan", guard(rbac.PermissionDomainLifecyclePlan, domainapi.LifecyclePlanHandler()))
	mux.Handle("/api/v1/inventory/normalize", guard(rbac.PermissionInventoryNormalize, inventoryapi.NormalizeHandler()))
	mux.Handle("/api/v1/inventory/reconcile", guard(rbac.PermissionInventoryReconcile, inventoryapi.ReconcileHandler()))
	mux.Handle("/api/v1/inventory/freshness", guard(rbac.PermissionInventoryFreshness, inventoryapi.FreshnessHandler()))
	mux.Handle("/api/v1/agent/enrollment/normalize", guard(rbac.PermissionAgentEnrollmentNormalize, agentapi.EnrollmentHandler()))
	mux.Handle("/api/v1/agent/heartbeat/evaluate", guard(rbac.PermissionAgentHeartbeatEvaluate, agentapi.HeartbeatHandler()))
	mux.Handle("/api/v1/agent/lease/evaluate", guard(rbac.PermissionAgentLeaseEvaluate, agentapi.LeaseHandler()))
	return mux
}
