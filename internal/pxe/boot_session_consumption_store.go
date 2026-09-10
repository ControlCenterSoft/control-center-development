package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	bootSessionConsumptionStoreRecordVersion = "boot-session-consumption-store-record-v1"
	bootSessionConsumptionStoreMaxBytes      = 64 * 1024
)

var ErrBootSessionConsumptionStore = errors.New("PXE boot session consumption store failed")

type DirectoryBootSessionConsumptionStore struct {
	root string
}

type bootSessionConsumptionStoreRecord struct {
	RecordVersion string                        `json:"recordVersion"`
	Receipt       BootSessionConsumptionReceipt `json:"receipt"`
}

func NewDirectoryBootSessionConsumptionStore(root string) (*DirectoryBootSessionConsumptionStore, error) {
	if root != strings.TrimSpace(root) || root == "" {
		return nil, fmt.Errorf("%w: storage root must be a non-empty canonical path", ErrBootSessionConsumptionStore)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve storage root: %v", ErrBootSessionConsumptionStore, err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o700); err != nil {
		return nil, fmt.Errorf("%w: create storage root: %v", ErrBootSessionConsumptionStore, err)
	}
	info, err := os.Lstat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: inspect storage root: %v", ErrBootSessionConsumptionStore, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: storage root must be a real directory", ErrBootSessionConsumptionStore)
	}
	return &DirectoryBootSessionConsumptionStore{root: absoluteRoot}, nil
}

func (s *DirectoryBootSessionConsumptionStore) AtomicConsumeBootSession(
	candidate BootSessionConsumptionReceipt,
) (BootSessionConsumptionStoreResult, error) {
	if s == nil || s.root == "" {
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: initialized store is required", ErrBootSessionConsumptionStore)
	}
	if err := validateBootSessionConsumptionStoreReceipt(candidate); err != nil {
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous}, err
	}

	recordBytes, err := json.Marshal(bootSessionConsumptionStoreRecord{
		RecordVersion: bootSessionConsumptionStoreRecordVersion,
		Receipt:       candidate,
	})
	if err != nil {
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: encode durable record: %v", ErrBootSessionConsumptionStore, err)
	}
	if len(recordBytes) > bootSessionConsumptionStoreMaxBytes {
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: durable record exceeds bounded size", ErrBootSessionConsumptionStore)
	}
	recordBytes = append(recordBytes, '\n')

	finalPath := filepath.Join(s.root, candidate.ReplayKey+".json")
	temporary, err := os.CreateTemp(s.root, ".boot-session-consume-")
	if err != nil {
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: create temporary record: %v", ErrBootSessionConsumptionStore, err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: restrict temporary record permissions: %v", ErrBootSessionConsumptionStore, err)
	}
	if _, err := temporary.Write(recordBytes); err != nil {
		_ = temporary.Close()
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: write temporary record: %v", ErrBootSessionConsumptionStore, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: sync temporary record: %v", ErrBootSessionConsumptionStore, err)
	}
	if err := temporary.Close(); err != nil {
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: close temporary record: %v", ErrBootSessionConsumptionStore, err)
	}

	if err := os.Link(temporaryPath, finalPath); err != nil {
		if errors.Is(err, fs.ErrExist) {
			existing, loadErr := s.loadExisting(candidate.ReplayKey)
			if loadErr != nil {
				return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous}, loadErr
			}
			return BootSessionConsumptionStoreResult{
				State:    BootSessionConsumptionAlreadyConsumed,
				Existing: &existing,
			}, nil
		}
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
			fmt.Errorf("%w: atomically publish durable record: %v", ErrBootSessionConsumptionStore, err)
	}

	if err := syncBootSessionConsumptionStoreDirectory(s.root); err != nil {
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous}, err
	}
	return BootSessionConsumptionStoreResult{State: BootSessionConsumptionCommitted}, nil
}

func (s *DirectoryBootSessionConsumptionStore) loadExisting(
	replayKey string,
) (BootSessionConsumptionReceipt, error) {
	finalPath := filepath.Join(s.root, replayKey+".json")
	info, err := os.Lstat(finalPath)
	if err != nil {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: inspect existing record: %v", ErrBootSessionConsumptionStore, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: existing record must be a regular file", ErrBootSessionConsumptionStore)
	}
	if info.Size() <= 0 || info.Size() > bootSessionConsumptionStoreMaxBytes {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: existing record size is invalid", ErrBootSessionConsumptionStore)
	}

	file, err := os.Open(finalPath)
	if err != nil {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: open existing record: %v", ErrBootSessionConsumptionStore, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, bootSessionConsumptionStoreMaxBytes+1))
	decoder.DisallowUnknownFields()
	var record bootSessionConsumptionStoreRecord
	if err := decoder.Decode(&record); err != nil {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: decode existing record: %v", ErrBootSessionConsumptionStore, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: existing record contains trailing data", ErrBootSessionConsumptionStore)
	}
	if record.RecordVersion != bootSessionConsumptionStoreRecordVersion {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: unsupported record version", ErrBootSessionConsumptionStore)
	}
	if record.Receipt.ReplayKey != replayKey {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: existing record replay-key binding changed", ErrBootSessionConsumptionStore)
	}
	if err := validateBootSessionConsumptionStoreReceipt(record.Receipt); err != nil {
		return BootSessionConsumptionReceipt{}, err
	}
	return record.Receipt, nil
}

func validateBootSessionConsumptionStoreReceipt(receipt BootSessionConsumptionReceipt) error {
	if receipt.ReceiptVersion != bootSessionConsumptionReceiptVersion ||
		!receipt.SingleUse ||
		!receipt.AtomicConsumeRequired ||
		!receipt.BootHandoffConsumed ||
		receipt.ProvisioningAuthorized ||
		receipt.SecretInjectionAuthorized ||
		receipt.HostMutation ||
		receipt.NetworkMutation ||
		receipt.ReplayAuthorized ||
		receipt.RollbackAction != "expire-consumed-session-before-provisioning" ||
		receipt.RecoveryAction != "reissue-new-session-from-fresh-serving-evidence" {
		return fmt.Errorf("%w: receipt safety contract does not permit durable consumption", ErrBootSessionConsumptionStore)
	}
	for _, value := range []string{
		receipt.ReceiptID,
		receipt.AdmissionID,
		receipt.ReplayKey,
		receipt.PlanID,
		receipt.ServingReceiptID,
		receipt.MediaSHA256,
	} {
		if !bootSessionConsumptionCanonicalSHA256(value) {
			return fmt.Errorf("%w: receipt contains non-canonical digest identity", ErrBootSessionConsumptionStore)
		}
	}
	for _, value := range []string{receipt.RequestID, receipt.MachineID, receipt.AttemptID, receipt.ConsumerID} {
		if !validBootSessionIdentity(value) {
			return fmt.Errorf("%w: receipt contains invalid bounded identity", ErrBootSessionConsumptionStore)
		}
	}
	if receipt.MediaSize <= 0 || receipt.ConsumedAtUnix <= 0 {
		return fmt.Errorf("%w: receipt contains invalid media or consumption bounds", ErrBootSessionConsumptionStore)
	}

	digestInput := bootSessionConsumptionReceiptDigest{
		ReceiptVersion:   receipt.ReceiptVersion,
		AdmissionID:      receipt.AdmissionID,
		ReplayKey:        receipt.ReplayKey,
		RequestID:        receipt.RequestID,
		MachineID:        receipt.MachineID,
		AttemptID:        receipt.AttemptID,
		ConsumerID:       receipt.ConsumerID,
		PlanID:           receipt.PlanID,
		ServingReceiptID: receipt.ServingReceiptID,
		MediaSHA256:      receipt.MediaSHA256,
		MediaSize:        receipt.MediaSize,
		Target:           receipt.Target,
		ConsumedAtUnix:   receipt.ConsumedAtUnix,
		RollbackAction:   receipt.RollbackAction,
		RecoveryAction:   receipt.RecoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return fmt.Errorf("%w: encode receipt integrity digest: %v", ErrBootSessionConsumptionStore, err)
	}
	digest := sha256.Sum256(encoded)
	if receipt.ReceiptID != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("%w: receipt integrity digest does not match durable evidence", ErrBootSessionConsumptionStore)
	}
	return nil
}

func syncBootSessionConsumptionStoreDirectory(root string) error {
	directory, err := os.Open(root)
	if err != nil {
		return fmt.Errorf("%w: open storage directory for durability sync: %v", ErrBootSessionConsumptionStore, err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("%w: sync storage directory: %v", ErrBootSessionConsumptionStore, err)
	}
	return nil
}
