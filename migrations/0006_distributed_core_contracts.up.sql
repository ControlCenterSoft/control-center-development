BEGIN;

CREATE TABLE IF NOT EXISTS cc_scopes (
    id text PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('global','site','management-zone')),
    parent_id text REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    owner_scope text NOT NULL,
    display_name text NOT NULL,
    generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
    resource_version bigint NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK ((kind = 'global' AND parent_id IS NULL) OR kind <> 'global')
);
CREATE INDEX IF NOT EXISTS cc_scopes_parent_idx ON cc_scopes(parent_id);

CREATE TABLE IF NOT EXISTS cc_nodes_v2 (
    object_id text PRIMARY KEY,
    legacy_node_id text UNIQUE,
    scope_id text NOT NULL REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    owner_scope text NOT NULL,
    display_name text NOT NULL,
    lifecycle_state text NOT NULL CHECK (lifecycle_state IN ('pending','enrolled','active','draining','maintenance','replacing','removed')),
    roles text[] NOT NULL CHECK (cardinality(roles) > 0),
    site_id text,
    management_zone_id text,
    enrollment_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
    resource_version bigint NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS cc_nodes_v2_scope_idx ON cc_nodes_v2(scope_id, lifecycle_state);
CREATE INDEX IF NOT EXISTS cc_nodes_v2_roles_idx ON cc_nodes_v2 USING gin(roles);

CREATE TABLE IF NOT EXISTS cc_desired_object_states (
    object_id text PRIMARY KEY,
    scope_id text NOT NULL REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    owner_scope text NOT NULL,
    kind text NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    resource_version bigint NOT NULL CHECK (resource_version > 0),
    spec jsonb NOT NULL,
    requested_by text NOT NULL,
    requested_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS cc_actual_object_states (
    object_id text PRIMARY KEY,
    scope_id text NOT NULL REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    owner_scope text NOT NULL,
    kind text NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    resource_version bigint NOT NULL CHECK (resource_version > 0),
    observed_generation bigint NOT NULL CHECK (observed_generation >= 0),
    status text NOT NULL,
    details jsonb,
    observed_at timestamptz NOT NULL,
    CHECK (observed_generation <= generation)
);

CREATE TABLE IF NOT EXISTS cc_object_ownership_claims (
    object_id text PRIMARY KEY,
    scope_id text NOT NULL REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    controller_id text NOT NULL,
    lease_id text NOT NULL UNIQUE,
    lease_until timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS cc_object_ownership_lease_idx ON cc_object_ownership_claims(lease_until);

CREATE TABLE IF NOT EXISTS cc_capacity_observations (
    id bigserial PRIMARY KEY,
    node_id text NOT NULL,
    scope_id text REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    resource text NOT NULL CHECK (resource IN ('cpu','memory','storage','network')),
    used double precision NOT NULL CHECK (used >= 0),
    capacity double precision NOT NULL CHECK (capacity > 0),
    observed_at timestamptz NOT NULL,
    CHECK (used <= capacity)
);
CREATE INDEX IF NOT EXISTS cc_capacity_observations_node_time_idx ON cc_capacity_observations(node_id, observed_at DESC);

CREATE TABLE IF NOT EXISTS cc_capacity_profiles (
    profile_id text PRIMARY KEY,
    scope_id text REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    workload text NOT NULL,
    required jsonb NOT NULL,
    safety_reserve jsonb NOT NULL DEFAULT '{}'::jsonb,
    generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
    updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS cc_capacity_constraints (
    id text PRIMARY KEY,
    scope_id text REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    resource text NOT NULL CHECK (resource IN ('cpu','memory','storage','network')),
    maximum_utilization double precision NOT NULL CHECK (maximum_utilization > 0 AND maximum_utilization <= 1),
    reason text,
    updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS cc_capacity_recommendations (
    id text PRIMARY KEY,
    node_id text NOT NULL,
    scope_id text REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    action text NOT NULL,
    resource text NOT NULL CHECK (resource IN ('cpu','memory','storage','network')),
    safe_additional double precision NOT NULL CHECK (safe_additional >= 0),
    confidence double precision NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    reason text NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS cc_recovery_points (
    id text PRIMARY KEY,
    scope_id text NOT NULL REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('automatic','manual','pre-change')),
    object_refs text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL,
    expires_at timestamptz,
    CHECK (expires_at IS NULL OR expires_at > created_at)
);

CREATE TABLE IF NOT EXISTS cc_backups_v2 (
    backup_id text PRIMARY KEY,
    recovery_point_id text NOT NULL REFERENCES cc_recovery_points(id) ON DELETE RESTRICT,
    repository_id text NOT NULL,
    state text NOT NULL CHECK (state IN ('pending','complete','failed')),
    checksum text,
    created_at timestamptz NOT NULL,
    completed_at timestamptz,
    CHECK (completed_at IS NULL OR completed_at >= created_at)
);

CREATE TABLE IF NOT EXISTS cc_restore_intents (
    restore_id text PRIMARY KEY,
    source_recovery_point_id text NOT NULL REFERENCES cc_recovery_points(id) ON DELETE RESTRICT,
    target_scope_id text NOT NULL REFERENCES cc_scopes(id) ON DELETE RESTRICT,
    mode text NOT NULL CHECK (mode IN ('object','point-in-time','full')),
    dry_run boolean NOT NULL DEFAULT true,
    requested_by text NOT NULL,
    requested_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS cc_market_manifests_v2 (
    module_id text NOT NULL,
    version text NOT NULL,
    display_name text NOT NULL,
    schema_version text NOT NULL CHECK (schema_version = '2'),
    roles text[] NOT NULL DEFAULT '{}',
    dependencies jsonb NOT NULL DEFAULT '[]'::jsonb,
    lifecycle jsonb NOT NULL,
    capacity jsonb NOT NULL DEFAULT '[]'::jsonb,
    recovery jsonb NOT NULL,
    network jsonb NOT NULL,
    generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
    resource_version bigint NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (module_id, version)
);

INSERT INTO cc_rbac_permissions (name, description) VALUES
    ('core.scopes.read','Read distributed scope hierarchy'),
    ('core.scopes.write','Manage distributed scope hierarchy'),
    ('core.nodes.contract.read','Read distributed node contracts'),
    ('core.nodes.contract.write','Manage distributed node contracts'),
    ('core.state.read','Read desired and actual object state'),
    ('core.state.write','Write desired object state'),
    ('capacity.read','Read capacity observations and recommendations'),
    ('capacity.write','Write capacity observations and profiles'),
    ('recovery.read','Read recovery metadata'),
    ('recovery.write','Create recovery points and restore intents'),
    ('market.manifests.v2.read','Read Market Manifest v2 contracts'),
    ('market.manifests.v2.write','Manage Market Manifest v2 contracts')
ON CONFLICT (name) DO NOTHING;

COMMIT;
