package postgres

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestPostgresAuditDetailsCanonicalizationPreservesExactNumbers(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	input := map[string]any{
		"large_integer": json.Number("9007199254740993"),
		"fraction":      json.Number("1.2300"),
		"exponent":      json.Number("1.25e3"),
	}
	canonical, err := canonicalizeAuditDetailsForPostgres(ctx, tx, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"large_integer", "fraction", "exponent"} {
		if _, ok := canonical[key].(json.Number); !ok {
			t.Fatalf("canonical %s decoded as %T, want json.Number", key, canonical[key])
		}
	}
	if got := canonical["large_integer"].(json.Number).String(); got != "9007199254740993" {
		t.Fatalf("large integer lost precision: %q", got)
	}
	if got := canonical["exponent"].(json.Number).String(); got == "1.25e3" {
		t.Fatalf("PostgreSQL jsonb exponent form was not canonicalized: %q", got)
	}

	second, err := canonicalizeAuditDetailsForPostgres(ctx, tx, canonical)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("PostgreSQL jsonb canonicalization is not idempotent: first=%s second=%s", firstJSON, secondJSON)
	}
}
