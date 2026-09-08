package domain

import "errors"

type Provider string

const (
	ProviderAuto    Provider = "auto"
	ProviderSamba   Provider = "samba-ad-dc"
	ProviderFreeIPA Provider = "freeipa"
)

var ErrIncompatibleProvider = errors.New("domain provider is incompatible with requirements")

type Requirements struct {
	WindowsDomainJoin bool
	GroupPolicy       bool
}

func ResolveProvider(preferred Provider, requirements Requirements) (Provider, error) {
	switch preferred {
	case "", ProviderAuto:
		if requirements.WindowsDomainJoin || requirements.GroupPolicy {
			return ProviderSamba, nil
		}
		return ProviderFreeIPA, nil
	case ProviderSamba:
		return ProviderSamba, nil
	case ProviderFreeIPA:
		if requirements.WindowsDomainJoin || requirements.GroupPolicy {
			return "", ErrIncompatibleProvider
		}
		return ProviderFreeIPA, nil
	default:
		return "", ErrIncompatibleProvider
	}
}
