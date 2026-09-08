package domain

import (
	"fmt"
	"strings"
)

// DirectoryJoinRequest describes a host enrollment into a selected identity provider.
type DirectoryJoinRequest struct {
	Platform   string
	Provider   string
	DomainName string
}

// ValidateDirectoryJoin enforces provider/platform compatibility.
func ValidateDirectoryJoin(request DirectoryJoinRequest) error {
	platform := strings.ToLower(strings.TrimSpace(request.Platform))
	provider := strings.ToLower(strings.TrimSpace(request.Provider))
	domainName := strings.TrimSpace(request.DomainName)

	if domainName == "" {
		return fmt.Errorf("domain name is required")
	}
	if platform != "windows" && platform != "linux" {
		return fmt.Errorf("unsupported platform %q", request.Platform)
	}
	if provider != "samba-ad" && provider != "freeipa" {
		return fmt.Errorf("unsupported provider %q", request.Provider)
	}
	if platform == "windows" && provider != "samba-ad" {
		return fmt.Errorf("windows domain join requires samba-ad")
	}
	return nil
}
