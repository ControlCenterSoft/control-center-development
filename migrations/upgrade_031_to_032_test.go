package migrations

import (
	"context"
	"strings"
	"testing"
	"time"

	postgresstore "control-center/internal/persistence/postgres"
)

var stable031Up = []string{
	"0001_initial.up.sql",
	"0002_local_identity_rbac_audit.up.sql",
	"0003_identity_persistence_invariants.up.sql",
	"0004_change_execution_core.up.sql",
	"0005_first_login_password_change.up.sql",
	"0006_distributed_core_objects.up.sql",
	"0007_network_contract_objects.up.sql",
	"0008_builtin_rbac_permissions.up.sql",
	"0009_auth_session_activity.up.sql",
	"0010_legacy_03_schema_compatibility.up.sql",
	"0011_job_timeline.up.sql",
	"0012_manual_job_retry_lineage.up.sql",
}

func TestPostgresUpgradeFrom031PreservesIdentityAndJobState(t *testing.T) {
	database := newDisposablePostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	applyMigrations(t, ctx, database.db, stable031Up)
	at := time.Date(2026, 9, 13, 1, 32, 0, 0, time.UTC)
	adminID, created, err := postgresstore.BootstrapAdmin(ctx, database.db, "admin", "hash-bootstrap-031", at.Add(-2*time.Minute))
	if err != nil || !created {
		t.Fatalf("bootstrap admin created=%v err=%v", created, err)
	}
	identities, err := postgresstore.NewIdentityStore(database.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.ChangePasswordAndRevokeSessions(ctx, adminID, "hash-bootstrap-031", "hash-selected-031", at.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	tx, err := database.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cc_config_revisions
(id,sequence,digest,content,created_at,created_by)
VALUES ('stable-031-revision',31,$1,'{"release":"0.31.0"}',$2,$3)`,
		"sha256:"+strings.Repeat("3", 64), at.Add(-50*time.Second), adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cc_policy_decisions
(id,policy_id,effect,risk,reason,minimum_approvals,distinct_actors,prohibit_requester,evaluated_at)
VALUES ('stable-031-policy-decision','stable-031-policy','allow','low','Stable 0.31 fixture',0,true,false,$1)`, at.Add(-45*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cc_changes
(id,action_name,requester,revision_id,decision_id,risk,input,idempotency_key,input_fingerprint,job_id,state,version,created_at,updated_at)
VALUES ('stable-031-change','resource.record',$1,'stable-031-revision','stable-031-policy-decision','low',
 '{"resourceId":"node-stable-031"}','stable-031-change-key',$2,'stable-031-job','failed',9,$3,$3)`,
		adminID, strings.Repeat("c", 64), at.Add(-40*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cc_jobs
(id,change_id,action_name,input,idempotency_key,input_fingerprint,status,attempt,max_attempts,version,created_at,updated_at)
VALUES ('stable-031-job','stable-031-change','resource.record','{"resourceId":"node-stable-031"}',
 'stable-031-job-key',$1,'failed',1,3,7,$2,$2)`, strings.Repeat("d", 64), at.Add(-30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cc_job_timeline
(job_id,job_version,event,status,attempt,occurred_at)
VALUES ('stable-031-job',7,'failed','failed',1,$1)`, at.Add(-30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	assertStable031State(t, ctx, database, adminID, false)
	applyMigrations(t, ctx, database.db, []string{"0013_incident_read_models.up.sql"})
	assertCurrentSchema(t, ctx, database.db)
	assertStable031State(t, ctx, database, adminID, true)

	insert032IncidentFixture(t, ctx, database, at)
	assert032IncidentRows(t, ctx, database, 1, 1)

	// Re-applying the additive 0.32 migration must be idempotent and must not
	// silently discard already-persisted 0.32 read-model state.
	applyMigrations(t, ctx, database.db, []string{"0013_incident_read_models.up.sql"})
	assertStable031State(t, ctx, database, adminID, true)
	assert032IncidentRows(t, ctx, database, 1, 1)

	// Rollback must remove only the 0.32 read models while preserving the
	// administrator-selected password and all Stable 0.31 durable Job state.
	applyMigrations(t, ctx, database.db, []string{"0013_incident_read_models.down.sql"})
	assertStable031State(t, ctx, database, adminID, false)

	// Forward recovery after rollback must recreate empty 0.32 read models;
	// rolled-back Incident state must not reappear from stale persistence.
	applyMigrations(t, ctx, database.db, []string{"0013_incident_read_models.up.sql"})
	assertCurrentSchema(t, ctx, database.db)
	assertStable031State(t, ctx, database, adminID, true)
	assert032IncidentRows(t, ctx, database, 0, 0)

	database.restart(t, ctx)
	assertCurrentSchema(t, ctx, database.db)
	assertStable031State(t, ctx, database, adminID, true)
	assert032IncidentRows(t, ctx, database, 0, 0)
}

func insert032IncidentFixture(t *testing.T, ctx context.Context, database *disposablePostgres, at time.Time) {
	t.Helper()
	if _, err := database.db.ExecContext(ctx, `INSERT INTO cc_incident_read_models
(object_id,scope_id,owner_scope,generation,resource_version,severity,status,title,started_at,last_observed_at,document,created_at,updated_at)
VALUES ('incident-upgrade-032','global','global',1,'incident-rv-032','warning','open','Upgrade rollback fixture',$1,$2,'{"source":"upgrade-qualification"}',$1,$2)`,
		at.Add(time.Second), at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `INSERT INTO cc_incident_affected_resources
(incident_id,resource_kind,resource_id,scope_id)
VALUES ('incident-upgrade-032','node','node-stable-031','global')`); err != nil {
		t.Fatal(err)
	}
}

func assert032IncidentRows(t *testing.T, ctx context.Context, database *disposablePostgres, wantIncidents, wantAffected int) {
	t.Helper()
	var incidents int
	if err := database.db.QueryRowContext(ctx, `SELECT count(*) FROM cc_incident_read_models`).Scan(&incidents); err != nil {
		t.Fatal(err)
	}
	if incidents != wantIncidents {
		t.Fatalf("0.32 incident row count=%d want=%d", incidents, wantIncidents)
	}
	var affected int
	if err := database.db.QueryRowContext(ctx, `SELECT count(*) FROM cc_incident_affected_resources`).Scan(&affected); err != nil {
		t.Fatal(err)
	}
	if affected != wantAffected {
		t.Fatalf("0.32 affected-resource row count=%d want=%d", affected, wantAffected)
	}
}

func assertStable031State(t *testing.T, ctx context.Context, database *disposablePostgres, adminID string, want032 bool) {
	t.Helper()
	var hash string
	var changeRequired bool
	if err := database.db.QueryRowContext(ctx, `SELECT password_hash,password_change_required FROM cc_local_users WHERE id=$1::uuid`, adminID).Scan(&hash, &changeRequired); err != nil {
		t.Fatal(err)
	}
	if hash != "hash-selected-031" || changeRequired {
		t.Fatal("Stable 0.31 local administrator state changed during upgrade")
	}

	var status string
	var version uint64
	if err := database.db.QueryRowContext(ctx, `SELECT status,version FROM cc_jobs WHERE id='stable-031-job'`).Scan(&status, &version); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || version != 7 {
		t.Fatalf("Stable 0.31 Job changed during upgrade: status=%q version=%d", status, version)
	}
	var timelineCount int
	if err := database.db.QueryRowContext(ctx, `SELECT count(*) FROM cc_job_timeline WHERE job_id='stable-031-job'`).Scan(&timelineCount); err != nil {
		t.Fatal(err)
	}
	if timelineCount != 1 {
		t.Fatalf("Stable 0.31 Job timeline changed during upgrade: count=%d", timelineCount)
	}

	for _, table := range []string{"cc_incident_read_models", "cc_incident_affected_resources"} {
		var exists bool
		if err := database.db.QueryRowContext(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists != want032 {
			t.Fatalf("0.32 table %s exists=%v want=%v", table, exists, want032)
		}
	}
}
