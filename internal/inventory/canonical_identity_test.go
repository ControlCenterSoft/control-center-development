package inventory

import (
	"reflect"
	"testing"
	"time"
)

func TestReconcileObservationsCanonicalizesIdentityFields(t *testing.T) {
	now := time.Date(2026, 9, 15, 7, 10, 0, 0, time.UTC)
	devices, err := ReconcileObservations([]DeviceObservation{
		{DeviceID: " device-1 ", Source: " agent ", Hostname: "node-old", SeenAt: now.Add(-time.Minute)},
		{DeviceID: "device-1", Source: "network", Hostname: "node-new", SeenAt: now},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := len(devices), 1; got != want {
		t.Fatalf("device count=%d want=%d", got, want)
	}
	if got, want := devices[0].DeviceID, "device-1"; got != want {
		t.Fatalf("device id=%q want=%q", got, want)
	}
	if got, want := devices[0].Latest.Source, "network"; got != want {
		t.Fatalf("latest source=%q want=%q", got, want)
	}
	if got, want := devices[0].Sources, []string{"agent", "network"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sources=%v want=%v", got, want)
	}
}

func TestMemoryRegistryGetAcceptsCanonicalEquivalentLookupID(t *testing.T) {
	now := time.Date(2026, 9, 15, 7, 10, 0, 0, time.UTC)
	registry := NewMemoryRegistry()
	if _, err := registry.Ingest([]DeviceObservation{{
		DeviceID: " device-1 ",
		Source:   " agent ",
		Hostname: "node-1",
		SeenAt:   now,
	}}); err != nil {
		t.Fatalf("unexpected ingest error: %v", err)
	}

	item, ok := registry.Get("  device-1  ")
	if !ok {
		t.Fatal("expected canonical-equivalent lookup to find device")
	}
	if got, want := item.DeviceID, "device-1"; got != want {
		t.Fatalf("device id=%q want=%q", got, want)
	}
	if got, want := item.Latest.Source, "agent"; got != want {
		t.Fatalf("latest source=%q want=%q", got, want)
	}
}
