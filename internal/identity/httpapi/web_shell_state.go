package httpapi

import "strings"

const productShellContractVersion = "cc.product-shell.v1"

type productShellSemanticState string

const (
	productShellStateCurrent     productShellSemanticState = "current"
	productShellStateUnavailable productShellSemanticState = "unavailable"
	productShellStateUnknown     productShellSemanticState = "unknown"
)

type productShellSignal struct {
	State     productShellSemanticState `json:"state"`
	Label     string                    `json:"label"`
	Detail    string                    `json:"detail,omitempty"`
	Confirmed bool                      `json:"confirmed"`
}

type productShellContext struct {
	Kind       string                    `json:"kind"`
	Label      string                    `json:"label"`
	State      productShellSemanticState `json:"state"`
	StateLabel string                    `json:"state_label"`
	Selected   bool                      `json:"selected"`
	Selectable bool                      `json:"selectable"`
}

type productShellNotifications struct {
	Connected bool   `json:"connected"`
	Count     *int   `json:"count,omitempty"`
	Label     string `json:"label"`
	Detail    string `json:"detail,omitempty"`
}

type productShellSnapshot struct {
	ContractVersion string                    `json:"contract_version"`
	Release         string                    `json:"release"`
	Language        string                    `json:"language"`
	Locale          string                    `json:"locale"`
	Environment     productShellSignal        `json:"environment"`
	Contexts        []productShellContext     `json:"contexts"`
	Health          productShellSignal        `json:"health"`
	Freshness       productShellSignal        `json:"freshness"`
	Risk            productShellSignal        `json:"risk"`
	Notifications   productShellNotifications `json:"notifications"`
}

// newProductShellSnapshot returns the 0.29 Product Web Shell state before
// authoritative runtime sources are wired into the shell. The contract is
// intentionally fail-closed: HTTP/API readiness must never be translated into
// healthy infrastructure, fresh evidence, low risk, or zero notifications.
func newProductShellSnapshot(release string) productShellSnapshot {
	release = strings.TrimSpace(release)
	if release == "" {
		release = "unknown"
	}

	return productShellSnapshot{
		ContractVersion: productShellContractVersion,
		Release:         release,
		Language:        "ru",
		Locale:          "ru-RU",
		Environment: productShellSignal{
			State:     productShellStateUnavailable,
			Label:     "Среда: не определена",
			Confirmed: false,
		},
		Contexts: []productShellContext{
			{
				Kind:       "installation",
				Label:      "Установка",
				State:      productShellStateCurrent,
				StateLabel: "Текущий контекст",
				Selected:   true,
				Selectable: false,
			},
			{
				Kind:       "site",
				Label:      "Сайт",
				State:      productShellStateUnavailable,
				StateLabel: "Не выбран",
				Selected:   false,
				Selectable: false,
			},
			{
				Kind:       "node",
				Label:      "Узел",
				State:      productShellStateUnavailable,
				StateLabel: "Не выбран",
				Selected:   false,
				Selectable: false,
			},
		},
		Health: productShellSignal{
			State:     productShellStateUnavailable,
			Label:     "Статус: данные не загружены",
			Detail:    "Интерфейс не подменяет фактический health-check.",
			Confirmed: false,
		},
		Freshness: productShellSignal{
			State:     productShellStateUnavailable,
			Label:     "Нет данных",
			Confirmed: false,
		},
		Risk: productShellSignal{
			State:     productShellStateUnknown,
			Label:     "Риск: неизвестен",
			Confirmed: false,
		},
		Notifications: productShellNotifications{
			Connected: false,
			Count:     nil,
			Label:     "Источник уведомлений не подключён",
			Detail:    "Количество активных событий не показывается без подтверждённого источника.",
		},
	}
}
