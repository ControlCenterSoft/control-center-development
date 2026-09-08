package agent

import (
	"errors"
	"sort"
	"strings"
)

var ErrInvalidEnrollment = errors.New("invalid agent enrollment request")

type EnrollmentRequest struct {
	NodeID       string
	Hostname     string
	Capabilities []string
}

func NormalizeEnrollment(request EnrollmentRequest) (EnrollmentRequest, error) {
	request.NodeID = strings.TrimSpace(request.NodeID)
	request.Hostname = strings.TrimSpace(request.Hostname)
	if request.NodeID == "" || request.Hostname == "" {
		return EnrollmentRequest{}, ErrInvalidEnrollment
	}

	seen := make(map[string]struct{}, len(request.Capabilities))
	capabilities := make([]string, 0, len(request.Capabilities))
	for _, capability := range request.Capabilities {
		capability = strings.ToLower(strings.TrimSpace(capability))
		if capability == "" {
			continue
		}
		if _, exists := seen[capability]; exists {
			continue
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)
	request.Capabilities = capabilities
	return request, nil
}
