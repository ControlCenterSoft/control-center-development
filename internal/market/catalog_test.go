package market

import "testing"

func TestBuiltinCatalogIsValidAndDeterministic(t *testing.T) {
	catalog := BuiltinCatalog()
	if len(catalog) != 7 {
		t.Fatalf("catalog size = %d, want 7", len(catalog))
	}
	for i, module := range catalog {
		if err := Validate(module); err != nil {
			t.Fatalf("module %s invalid: %v", module.ID, err)
		}
		if i > 0 && catalog[i-1].ID >= module.ID {
			t.Fatalf("catalog order is not deterministic: %s >= %s", catalog[i-1].ID, module.ID)
		}
	}
}

func TestDirectoryServicesOffersBothProviders(t *testing.T) {
	module, ok := Find("directory-services")
	if !ok {
		t.Fatal("directory-services module missing")
	}
	if len(module.Providers) != 2 || module.Providers[0] != "samba-ad-dc" || module.Providers[1] != "freeipa" {
		t.Fatalf("providers = %#v", module.Providers)
	}
}
