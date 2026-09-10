package pxe

import "errors"

var ErrBootSessionReadmission = errors.New("PXE boot session readmission validation failed")

const bootSessionReadmissionReceiptVersion = "boot-session-readmission-receipt-v1"

// BootSessionReadmissionReceipt seals recovery lineage after an exact
// definitely-absent consumption reconciliation. The receipt itself never
// authorizes boot, provisioning, secret injection, or host/network mutation;
// only the separately returned fresh BootSessionAdmission can authorize one
// bounded boot handoff.
type BootSessionReadmissionReceipt struct {
	ReceiptID                    string                        `json:"receiptId"`
	ReceiptVersion               string                        `json:"receiptVersion"`
	ReconciliationID             string                        `json:"reconciliationId"`
	CandidateReceiptID           string                        `json:"candidateReceiptId"`
	PreviousAdmissionID          string                        `json:"previousAdmissionId"`
	PreviousReplayKey            string                        `json:"previousReplayKey"`
	FreshAdmissionID             string                        `json:"freshAdmissionId"`
	FreshReplayKey               string                        `json:"freshReplayKey"`
	MachineID                    string                        `json:"machineId"`
	PlanID                       string                        `json:"planId"`
	ServingReceiptID             string                        `json:"servingReceiptId"`
	MediaSHA256                  string                        `json:"mediaSha256"`
	MediaSize                    int64                         `json:"mediaSize"`
	Target                       InstallMediaPublicationTarget `json:"target"`
	IssuedAtUnix                 int64                         `json:"issuedAtUnix"`
	ExpiresAtUnix                int64                         `json:"expiresAtUnix"`
	FreshServingEvidenceVerified bool                          `json:"freshServingEvidenceVerified"`
	FreshAdmissionIssued         bool                          `json:"freshAdmissionIssued"`
	PreviousAdmissionReusable    bool                          `json:"previousAdmissionReusable"`
	PreviousReplayAuthorized     bool                          `json:"previousReplayAuthorized"`
	BootHandoffAuthorized        bool                          `json:"bootHandoffAuthorized"`
	ProvisioningAuthorized       bool                          `json:"provisioningAuthorized"`
	SecretInjectionAuthorized    bool                          `json:"secretInjectionAuthorized"`
	HostMutation                 bool                          `json:"hostMutation"`
	NetworkMutation              bool                          `json:"networkMutation"`
	ProductionMutation           bool                          `json:"productionMutation"`
	RecoveryAction               string                        `json:"recoveryAction"`
}

type bootSessionReadmissionReceiptDigest struct {
	ReceiptVersion      string                        `json:"receiptVersion"`
	ReconciliationID    string                        `json:"reconciliationId"`
	CandidateReceiptID  string                        `json:"candidateReceiptId"`
	PreviousAdmissionID string                        `json:"previousAdmissionId"`
	PreviousReplayKey   string                        `json:"previousReplayKey"`
	FreshAdmissionID    string                        `json:"freshAdmissionId"`
	FreshReplayKey      string                        `json:"freshReplayKey"`
	MachineID           string                        `json:"machineId"`
	PlanID              string                        `json:"planId"`
	ServingReceiptID    string                        `json:"servingReceiptId"`
	MediaSHA256         string                        `json:"mediaSha256"`
	MediaSize           int64                         `json:"mediaSize"`
	Target              InstallMediaPublicationTarget `json:"target"`
	IssuedAtUnix        int64                         `json:"issuedAtUnix"`
	ExpiresAtUnix       int64                         `json:"expiresAtUnix"`
	RecoveryAction      string                        `json:"recoveryAction"`
}
