package market

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type LifecycleOperation string

const (
	Install LifecycleOperation = "install"
	Upgrade LifecycleOperation = "upgrade"
	Remove  LifecycleOperation = "remove"
)

type Module struct {
	ID           string               `json:"id"`
	Version      string               `json:"version"`
	Capabilities []string             `json:"capabilities"`
	Platforms    []string             `json:"platforms"`
	Providers    []string             `json:"providers,omitempty"`
	Lifecycle    []LifecycleOperation `json:"lifecycle"`
}

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func Validate(module Module) error {
	if strings.TrimSpace(module.ID) == "" || !versionPattern.MatchString(module.Version) {
		return errors.New("module id and semantic version are required")
	}
	if len(module.Capabilities) == 0 || len(module.Platforms) == 0 || len(module.Lifecycle) == 0 {
		return errors.New("module capabilities, platforms, and lifecycle are required")
	}
	seen := map[LifecycleOperation]bool{}
	for _, operation := range module.Lifecycle {
		if operation != Install && operation != Upgrade && operation != Remove {
			return fmt.Errorf("unsupported lifecycle operation %q", operation)
		}
		if seen[operation] {
			return fmt.Errorf("duplicate lifecycle operation %q", operation)
		}
		seen[operation] = true
	}
	return nil
}

func BuiltinCatalog() []Module {
	modules := []Module{
		{
			ID:           "directory-services",
			Version:      "0.1.0",
			Capabilities: []string{"identity", "directory", "policy"},
			Platforms:    []string{"linux"},
			Providers:    []string{"samba-ad-dc", "freeipa"},
			Lifecycle:    []LifecycleOperation{Install, Upgrade, Remove},
		},
		{
			ID:           "pxe-deployment",
			Version:      "0.1.0",
			Capabilities: []string{"windows-deployment", "linux-deployment"},
			Platforms:    []string{"linux"},
			Lifecycle:    []LifecycleOperation{Install, Upgrade, Remove},
		},
		{
			ID:           "software-automation",
			Version:      "0.1.0",
			Capabilities: []string{"package-management", "configuration"},
			Platforms:    []string{"linux", "windows"},
			Lifecycle:    []LifecycleOperation{Install, Upgrade, Remove},
		},
		{
			ID:           "endpoint-inventory",
			Version:      "0.1.0",
			Capabilities: []string{"inventory"},
			Platforms:    []string{"linux", "windows"},
			Lifecycle:    []LifecycleOperation{Install, Upgrade, Remove},
		},
		{
			ID:           "dns-dhcp",
			Version:      "0.1.0",
			Capabilities: []string{"dns", "dhcp"},
			Platforms:    []string{"linux"},
			Lifecycle:    []LifecycleOperation{Install, Upgrade, Remove},
		},
		{
			ID:           "file-services",
			Version:      "0.1.0",
			Capabilities: []string{"smb", "nfs"},
			Platforms:    []string{"linux"},
			Lifecycle:    []LifecycleOperation{Install, Upgrade, Remove},
		},
		{
			ID:           "monitoring",
			Version:      "0.1.0",
			Capabilities: []string{"metrics", "health"},
			Platforms:    []string{"linux", "windows"},
			Lifecycle:    []LifecycleOperation{Install, Upgrade, Remove},
		},
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].ID < modules[j].ID })
	return modules
}

func Find(id string) (Module, bool) {
	for _, module := range BuiltinCatalog() {
		if module.ID == id {
			return module, true
		}
	}
	return Module{}, false
}
