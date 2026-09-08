package rbac

import "testing"

func TestDenyByDefault(t *testing.T) {
	a := NewAuthorizer()
	if a.Allowed("user-1", PermissionOverviewRead, GlobalScope()) {
		t.Fatal("permission granted without role or binding")
	}
	for _, role := range BuiltinRoles() {
		if err := a.RegisterRole(role); err != nil {
			t.Fatal(err)
		}
	}
	if a.Allowed("user-1", PermissionOverviewRead, GlobalScope()) {
		t.Fatal("permission granted without explicit binding")
	}
}

func TestExactScopeAndGlobalScope(t *testing.T) {
	a := NewAuthorizer()
	if err := a.RegisterRole(Role{Name: "site-viewer", Permissions: []Permission{PermissionOverviewRead}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Bind(Binding{SubjectID: "user-1", RoleName: "site-viewer", Scope: Scope{Kind: ScopeSite, ID: "site-a"}}); err != nil {
		t.Fatal(err)
	}
	if !a.Allowed("user-1", PermissionOverviewRead, Scope{Kind: ScopeSite, ID: "site-a"}) {
		t.Fatal("exact scoped permission denied")
	}
	if a.Allowed("user-1", PermissionOverviewRead, Scope{Kind: ScopeSite, ID: "site-b"}) {
		t.Fatal("binding escaped its scope")
	}
	if a.Allowed("user-1", PermissionUsersWrite, Scope{Kind: ScopeSite, ID: "site-a"}) {
		t.Fatal("unassigned permission granted")
	}
}

func TestAdministratorStillRequiresBinding(t *testing.T) {
	a := NewAuthorizer()
	for _, role := range BuiltinRoles() {
		if err := a.RegisterRole(role); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Bind(Binding{SubjectID: "admin-1", RoleName: "administrator", Scope: GlobalScope()}); err != nil {
		t.Fatal(err)
	}
	if !a.Allowed("admin-1", PermissionRolesWrite, Scope{Kind: ScopeResource, ID: "resource-1"}) {
		t.Fatal("global administrator binding did not authorize target")
	}
	if a.Allowed("other", PermissionRolesWrite, GlobalScope()) {
		t.Fatal("administrator privilege leaked to another subject")
	}
}

func TestBuiltinRolesSeparateLifecycleReadAndPlan(t *testing.T) {
	a := NewAuthorizer()
	for _, role := range BuiltinRoles() {
		if err := a.RegisterRole(role); err != nil {
			t.Fatal(err)
		}
	}
	for _, roleName := range []string{"viewer", "auditor", "operator"} {
		if err := a.Bind(Binding{SubjectID: roleName, RoleName: roleName, Scope: GlobalScope()}); err != nil {
			t.Fatal(err)
		}
	}
	for _, readOnlyRole := range []string{"viewer", "auditor"} {
		if !a.Allowed(readOnlyRole, PermissionNodeLifecycleRead, GlobalScope()) {
			t.Fatalf("%s cannot read node lifecycle", readOnlyRole)
		}
		if a.Allowed(readOnlyRole, PermissionNodeLifecyclePlan, GlobalScope()) {
			t.Fatalf("%s can plan node lifecycle transition", readOnlyRole)
		}
	}
	if !a.Allowed("operator", PermissionNodeLifecycleRead, GlobalScope()) || !a.Allowed("operator", PermissionNodeLifecyclePlan, GlobalScope()) {
		t.Fatal("operator lacks lifecycle read or plan permission")
	}
	if a.Allowed("unbound", PermissionNodeLifecycleRead, GlobalScope()) || a.Allowed("unbound", PermissionNodeLifecyclePlan, GlobalScope()) {
		t.Fatal("lifecycle permission granted without a binding")
	}
}

func TestBuiltinDistributedCorePermissions(t *testing.T) {
	roles := make(map[string]map[Permission]struct{})
	for _, role := range BuiltinRoles() {
		permissions := make(map[Permission]struct{}, len(role.Permissions))
		for _, permission := range role.Permissions {
			permissions[permission] = struct{}{}
		}
		roles[role.Name] = permissions
	}
	for _, roleName := range []string{"operator", "auditor", "viewer"} {
		if _, allowed := roles[roleName][PermissionCoreObjectsRead]; !allowed {
			t.Errorf("%s lacks %s", roleName, PermissionCoreObjectsRead)
		}
	}
	if _, allowed := roles["operator"][PermissionCoreObjectsWrite]; !allowed {
		t.Errorf("operator lacks %s", PermissionCoreObjectsWrite)
	}
	for _, roleName := range []string{"auditor", "viewer"} {
		if _, allowed := roles[roleName][PermissionCoreObjectsWrite]; allowed {
			t.Errorf("%s unexpectedly has %s", roleName, PermissionCoreObjectsWrite)
		}
	}
}
