package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestDistributedCoreUpgradeMigrationIsAdditiveAndDeterministic(t *testing.T) {
	up := readMigration(t, "0006_distributed_core_objects.up.sql")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS cc_core_objects",
		"CREATE TABLE IF NOT EXISTS cc_core_object_mutations",
		"resource_version varchar(255) NOT NULL UNIQUE",
		"idempotency_key varchar(255) PRIMARY KEY",
		"request_fingerprint char(64) NOT NULL",
		"cc_core_object_mutations_resource_version_format",
		"jsonb_typeof(document) = 'object'",
		"'global', 'scope', 'global', 'global', 1",
		"'bootstrap-v0.3-global'",
		"ON CONFLICT (object_id) DO NOTHING",
		"'core.objects.read'",
		"'core.objects.write'",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0006 up migration lacks %q", required)
		}
	}
	upper := strings.ToUpper(up)
	for _, forbidden := range []string{
		"DROP TABLE ORGANIZATIONS",
		"DROP TABLE RESOURCES",
		"DROP TABLE CONFIG_REVISIONS",
		"ALTER TABLE ORGANIZATIONS",
		"ALTER TABLE RESOURCES",
		"ALTER TABLE CONFIG_REVISIONS",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("0006 upgrade is not additive: found %q", forbidden)
		}
	}
}

func TestDistributedCoreDownMigrationIsScoped(t *testing.T) {
	down := strings.ToUpper(readMigration(t, "0006_distributed_core_objects.down.sql"))
	if !strings.Contains(down, "DROP TABLE IF EXISTS CC_CORE_OBJECT_MUTATIONS") {
		t.Fatal("0006 down migration does not remove its receipt table")
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS CC_CORE_OBJECTS") {
		t.Fatal("0006 down migration does not remove its table")
	}
	for _, legacy := range []string{"ORGANIZATIONS", "RESOURCES", "CONFIG_REVISIONS"} {
		if strings.Contains(down, "DROP TABLE "+legacy) || strings.Contains(down, "ALTER TABLE "+legacy) {
			t.Fatalf("0006 down migration touches legacy table %s", legacy)
		}
	}
}

func TestPostgresUpgradeFrom03PreservesLegacyState(t *testing.T) {
	databaseURL := os.Getenv("MIGRATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MIGRATION_TEST_DATABASE_URL is not set; PostgreSQL migration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	// This test intentionally refuses to clean or overwrite an existing schema:
	// MIGRATION_TEST_DATABASE_URL must identify an isolated disposable database.
	var occupied bool
	if err := database.QueryRowContext(ctx, `SELECT
        to_regclass('public.organizations') IS NOT NULL OR
        to_regclass('public.cc_local_users') IS NOT NULL OR
        to_regclass('public.cc_core_objects') IS NOT NULL`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if occupied {
		t.Fatal("migration test database is not empty; refusing to modify it")
	}

	applyMigration(t, ctx, database, "0001_initial.up.sql")
	const organizationID = "00000000-0000-4000-8000-000000000301"
	const resourceID = "00000000-0000-4000-8000-000000000302"
	if _, err := database.ExecContext(ctx, `INSERT INTO organizations (id,slug,name)
VALUES ($1,'legacy-03','Legacy 0.3 organization')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO resources
(id,organization_id,kind,name,status,labels,specification,observed_state)
VALUES ($1,$2,'node','Legacy node','ready','{"site":"legacy"}','{"enabled":true}','{"health":"ok"}')`, resourceID, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO config_revisions
(organization_id,resource_id,revision,configuration,checksum_sha256,created_by)
VALUES ($1,$2,1,'{"enabled":true}',$3,'legacy-operator')`, organizationID, resourceID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"0002_local_identity_rbac_audit.up.sql",
		"0003_identity_persistence_invariants.up.sql",
		"0004_change_execution_core.up.sql",
		"0005_first_login_password_change.up.sql",
	} {
		applyMigration(t, ctx, database, name)
	}
	assertLegacyFixture(t, ctx, database, organizationID, resourceID)

	applyMigration(t, ctx, database, "0006_distributed_core_objects.up.sql")
	assertLegacyFixture(t, ctx, database, organizationID, resourceID)
	assertDistributedCoreBootstrap(t, ctx, database)
	var firstCreatedAt, firstUpdatedAt time.Time
	if err := database.QueryRowContext(ctx, `SELECT created_at,updated_at
FROM cc_core_objects WHERE object_id='global'`).Scan(&firstCreatedAt, &firstUpdatedAt); err != nil {
		t.Fatal(err)
	}

	// Reapplying the additive migration must not duplicate or rewrite bootstrap
	// state. This also exercises every IF NOT EXISTS/ON CONFLICT path.
	applyMigration(t, ctx, database, "0006_distributed_core_objects.up.sql")
	assertLegacyFixture(t, ctx, database, organizationID, resourceID)
	assertDistributedCoreBootstrap(t, ctx, database)
	var replayCreatedAt, replayUpdatedAt time.Time
	if err := database.QueryRowContext(ctx, `SELECT created_at,updated_at
FROM cc_core_objects WHERE object_id='global'`).Scan(&replayCreatedAt, &replayUpdatedAt); err != nil {
		t.Fatal(err)
	}
	if !replayCreatedAt.Equal(firstCreatedAt) || !replayUpdatedAt.Equal(firstUpdatedAt) {
		t.Fatalf("replayed migration rewrote bootstrap timestamps: before=(%s,%s) after=(%s,%s)", firstCreatedAt, firstUpdatedAt, replayCreatedAt, replayUpdatedAt)
	}

	applyMigration(t, ctx, database, "0006_distributed_core_objects.down.sql")
	assertLegacyFixture(t, ctx, database, organizationID, resourceID)
	var coreTablesGone bool
	if err := database.QueryRowContext(ctx, `SELECT
        to_regclass('public.cc_core_objects') IS NULL AND
        to_regclass('public.cc_core_object_mutations') IS NULL`).Scan(&coreTablesGone); err != nil {
		t.Fatal(err)
	}
	if !coreTablesGone {
		t.Fatal("scoped down migration left distributed core tables behind")
	}

	// Leave the disposable database at the cumulative schema so repository and
	// orchestration integration tests can run against the same CI service.
	applyMigration(t, ctx, database, "0006_distributed_core_objects.up.sql")
	assertDistributedCoreBootstrap(t, ctx, database)
}

func applyMigration(t *testing.T, ctx context.Context, database *sql.DB, name string) {
	t.Helper()
	content := readMigration(t, name)
	if _, err := database.ExecContext(ctx, content); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
}

func assertLegacyFixture(t *testing.T, ctx context.Context, database *sql.DB, organizationID, resourceID string) {
	t.Helper()
	var count int
	if err := database.QueryRowContext(ctx, `SELECT count(*)
FROM organizations AS organization
JOIN resources AS resource ON resource.organization_id=organization.id
JOIN config_revisions AS revision ON revision.resource_id=resource.id
WHERE organization.id=$1 AND organization.slug='legacy-03'
  AND resource.id=$2 AND resource.status='ready'
  AND resource.labels='{"site":"legacy"}'::jsonb
  AND resource.specification='{"enabled":true}'::jsonb
  AND resource.observed_state='{"health":"ok"}'::jsonb
  AND revision.revision=1 AND revision.configuration='{"enabled":true}'::jsonb`, organizationID, resourceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("legacy 0.3 fixture count=%d, want 1", count)
	}
}

func assertDistributedCoreBootstrap(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	var valid bool
	if err := database.QueryRowContext(ctx, `SELECT count(*)=1
  AND bool_and(object_type='scope')
  AND bool_and(scope_id='global')
  AND bool_and(owner_scope='global')
  AND bool_and(generation=1)
  AND bool_and(resource_version='bootstrap-v0.3-global')
  AND bool_and(document='{"id":"global","kind":"global","name":"Global"}'::jsonb)
FROM cc_core_objects WHERE object_id='global'`).Scan(&valid); err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("distributed core bootstrap object is missing or non-canonical")
	}
	var permissionCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM cc_rbac_permissions
WHERE name IN ('core.objects.read','core.objects.write')`).Scan(&permissionCount); err != nil {
		t.Fatal(err)
	}
	if permissionCount != 2 {
		t.Fatalf("distributed core permission count=%d, want 2", permissionCount)
	}
}

func readMigration(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
