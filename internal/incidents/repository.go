package incidents

import (
	"context"
	"errors"
)

// ErrNotFound is returned when an incident object does not exist in the
// caller-visible repository scope. API adapters should avoid distinguishing a
// missing object from one hidden by authorization policy.
var ErrNotFound = errors.New("incident not found")

// Reader is the minimal persistence boundary required by read-only incident
// API/UI adapters. Authorization and scope derivation happen before this
// interface is called; implementations still validate all returned objects.
type Reader interface {
	Get(context.Context, string) (Incident, error)
	List(context.Context, ListQuery) (ListPage, error)
}
