package corecontracts

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestActualStateReplacementRejectsDivergentSameObservationIdentity(t *testing.T) {
	repository := newDeterministicObjectRepository(t)
	created := applyObject(t, repository, MutationRequest{
		Operation: MutationCreate, ObjectType: ObjectActualState,
		ObjectID: "actual-observation-a", ScopeID: "global", OwnerScope: "global",
		Document: json.RawMessage(`{"kind":"service.config","target_object_id":"service-a","desired_object_id":"desired-a","observed_generation":2,"source_node_id":"node-a","observed_at":"2026-09-15T12:00:00Z","status":"progressing","state":{"healthy":false}}`),
	})

	request := MutationRequest{
		Operation: MutationReplace, ObjectType: ObjectActualState,
		ObjectID: created.ObjectID, ScopeID: created.ScopeID, OwnerScope: created.OwnerScope,
		Document: json.RawMessage(`{"kind":"service.config","target_object_id":"service-a","desired_object_id":"desired-a","observed_generation":2,"source_node_id":"node-a","observed_at":"2026-09-15T12:00:00Z","status":"converged","state":{"healthy":true}}`),
		Precondition: objectPrecondition(created),
	}

	if _, err := repository.Apply(context.Background(), request, nextMutationKey()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("same observation identity divergence error = %v, want ErrInvalidTransition", err)
	}
	stored, err := repository.Get(context.Background(), created.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ResourceVersion != created.ResourceVersion || string(stored.Document) != string(created.Document) {
		t.Fatalf("rejected observation divergence changed stored object: before=%#v after=%#v", created, stored)
	}
}

func TestActualStateReplacementAllowsEquivalentSameObservationIdentity(t *testing.T) {
	repository := newDeterministicObjectRepository(t)
	created := applyObject(t, repository, MutationRequest{
		Operation: MutationCreate, ObjectType: ObjectActualState,
		ObjectID: "actual-observation-b", ScopeID: "global", OwnerScope: "global",
		Document: json.RawMessage(`{"kind":"service.config","target_object_id":"service-b","desired_object_id":"desired-b","observed_generation":3,"source_node_id":"node-b","observed_at":"2026-09-15T12:05:00Z","status":"converged","state":{"a":1,"b":2}}`),
	})

	replaced := applyObject(t, repository, MutationRequest{
		Operation: MutationReplace, ObjectType: ObjectActualState,
		ObjectID: created.ObjectID, ScopeID: created.ScopeID, OwnerScope: created.OwnerScope,
		Document: json.RawMessage(`{"state":{"b":2,"a":1},"status":"converged","observed_at":"2026-09-15T12:05:00Z","source_node_id":"node-b","observed_generation":3,"desired_object_id":"desired-b","target_object_id":"service-b","kind":"service.config"}`),
		Precondition: objectPrecondition(created),
	})

	if replaced.Generation != created.Generation {
		t.Fatalf("equivalent observation advanced semantic generation: before=%d after=%d", created.Generation, replaced.Generation)
	}
	if replaced.ResourceVersion == created.ResourceVersion {
		t.Fatalf("equivalent observation replacement did not allocate a new resource version")
	}
}
