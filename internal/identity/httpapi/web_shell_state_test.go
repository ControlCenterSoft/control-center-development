package httpapi

import "testing"

func TestProductShellSnapshotStartsFailClosed(t *testing.T) {
	snapshot := newProductShellSnapshot(" 0.29.0-test ")

	if snapshot.ContractVersion != productShellContractVersion {
		t.Fatalf("contract=%q", snapshot.ContractVersion)
	}
	if snapshot.Release != "0.29.0-test" {
		t.Fatalf("release=%q", snapshot.Release)
	}
	if snapshot.Language != "ru" || snapshot.Locale != "ru-RU" {
		t.Fatalf("locale=%s language=%s", snapshot.Locale, snapshot.Language)
	}

	for name, signal := range map[string]productShellSignal{
		"environment": snapshot.Environment,
		"health":      snapshot.Health,
		"freshness":   snapshot.Freshness,
		"risk":        snapshot.Risk,
	} {
		if signal.Confirmed {
			t.Fatalf("%s signal must not be confirmed without authoritative evidence", name)
		}
		if signal.State == "healthy" || signal.State == "fresh" || signal.State == "low" {
			t.Fatalf("%s signal invented a positive operational state: %q", name, signal.State)
		}
	}

	if snapshot.Health.State != productShellStateUnavailable {
		t.Fatalf("health state=%q", snapshot.Health.State)
	}
	if snapshot.Freshness.State != productShellStateUnavailable {
		t.Fatalf("freshness state=%q", snapshot.Freshness.State)
	}
	if snapshot.Risk.State != productShellStateUnknown {
		t.Fatalf("risk state=%q", snapshot.Risk.State)
	}
	if snapshot.Notifications.Connected || snapshot.Notifications.Count != nil {
		t.Fatal("disconnected notification source must not be represented as a confirmed zero count")
	}
}

func TestProductShellContextContractDoesNotInventSiteOrNodeSelection(t *testing.T) {
	snapshot := newProductShellSnapshot("0.29.0-test")
	if len(snapshot.Contexts) != 3 {
		t.Fatalf("contexts=%d", len(snapshot.Contexts))
	}

	selected := 0
	for _, context := range snapshot.Contexts {
		if context.Selected {
			selected++
			if context.Kind != "installation" || context.State != productShellStateCurrent {
				t.Fatalf("unexpected selected context: %#v", context)
			}
		}
		if context.Selectable {
			t.Fatalf("context %q became selectable without an authoritative selector backend", context.Kind)
		}
		if (context.Kind == "site" || context.Kind == "node") && context.State != productShellStateUnavailable {
			t.Fatalf("context %q invented availability: %#v", context.Kind, context)
		}
	}
	if selected != 1 {
		t.Fatalf("selected contexts=%d", selected)
	}
}

func TestProductShellSnapshotUsesExplicitUnknownReleaseFallback(t *testing.T) {
	if got := newProductShellSnapshot(" \t\n ").Release; got != "unknown" {
		t.Fatalf("release=%q", got)
	}
}
