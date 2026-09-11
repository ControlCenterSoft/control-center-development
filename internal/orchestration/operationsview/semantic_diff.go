package operationsview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	orchestrationconfig "control-center/internal/orchestration/config"
)

const (
	SemanticDiffContractVersion = "ui.operations-semantic-diff/v1"
	MaxSemanticDiffEntries      = 256
	MaxSemanticDiffPathLength   = 2048
	MaxSemanticDiffIdentifierLength = 255
)

var ErrInvalidSemanticDiff = errors.New("invalid operations semantic diff")

type SemanticDiffKind string

const (
	SemanticDiffAdded   SemanticDiffKind = "added"
	SemanticDiffRemoved SemanticDiffKind = "removed"
	SemanticDiffChanged SemanticDiffKind = "changed"
)

type SemanticDiff struct {
	ContractVersion  string              `json:"contract_version"`
	BaseRevisionID   string              `json:"base_revision_id"`
	TargetRevisionID string              `json:"target_revision_id"`
	BaseDigest       string              `json:"base_digest"`
	TargetDigest     string              `json:"target_digest"`
	Changes          []SemanticDiffEntry `json:"changes"`
}

type SemanticDiffEntry struct {
	Path string           `json:"path"`
	Kind SemanticDiffKind `json:"kind"`
}

// BuildSemanticDiff returns a deterministic structural diff between immutable
// configuration revisions. Values are deliberately omitted from the contract:
// configuration may contain credentials or other sensitive material, while the
// 0.31 operator UI only needs authoritative change shape for review. Large
// diffs fail closed instead of silently truncating approval evidence.
func BuildSemanticDiff(base, target orchestrationconfig.Revision) (SemanticDiff, error) {
	if err := validateSemanticDiffRevisionIdentity(base.ID(), base.Digest()); err != nil {
		return SemanticDiff{}, fmt.Errorf("%w: base revision: %v", ErrInvalidSemanticDiff, err)
	}
	if err := validateSemanticDiffRevisionIdentity(target.ID(), target.Digest()); err != nil {
		return SemanticDiff{}, fmt.Errorf("%w: target revision: %v", ErrInvalidSemanticDiff, err)
	}
	baseValue, err := decodeJSONObject(base.Content())
	if err != nil {
		return SemanticDiff{}, fmt.Errorf("%w: base revision: %v", ErrInvalidSemanticDiff, err)
	}
	targetValue, err := decodeJSONObject(target.Content())
	if err != nil {
		return SemanticDiff{}, fmt.Errorf("%w: target revision: %v", ErrInvalidSemanticDiff, err)
	}

	view := SemanticDiff{
		ContractVersion:  SemanticDiffContractVersion,
		BaseRevisionID:   base.ID(),
		TargetRevisionID: target.ID(),
		BaseDigest:       base.Digest(),
		TargetDigest:     target.Digest(),
		Changes:          []SemanticDiffEntry{},
	}
	if err := collectSemanticDiff("", baseValue, targetValue, &view.Changes); err != nil {
		return SemanticDiff{}, err
	}
	sort.Slice(view.Changes, func(i, j int) bool {
		if view.Changes[i].Path != view.Changes[j].Path {
			return view.Changes[i].Path < view.Changes[j].Path
		}
		return view.Changes[i].Kind < view.Changes[j].Kind
	})
	return view, nil
}

func validateSemanticDiffRevisionIdentity(id, digest string) error {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" || id != trimmedID || len(id) > MaxSemanticDiffIdentifierLength {
		return errors.New("canonical revision id is required")
	}
	if strings.TrimSpace(digest) == "" {
		return errors.New("revision digest is required")
	}
	return nil
}

func decodeJSONObject(content []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("configuration must be a JSON object")
	}
	if value == nil {
		return nil, errors.New("configuration must be a JSON object")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("configuration contains trailing JSON values")
	}
	return value, nil
}

func collectSemanticDiff(path string, before, after any, changes *[]SemanticDiffEntry) error {
	beforeObject, beforeIsObject := before.(map[string]any)
	afterObject, afterIsObject := after.(map[string]any)
	if beforeIsObject && afterIsObject {
		keys := make(map[string]struct{}, len(beforeObject)+len(afterObject))
		for key := range beforeObject {
			keys[key] = struct{}{}
		}
		for key := range afterObject {
			keys[key] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			beforeValue, beforeExists := beforeObject[key]
			afterValue, afterExists := afterObject[key]
			childPath := path + "/" + escapeJSONPointer(key)
			switch {
			case !beforeExists:
				if err := appendSemanticDiff(changes, SemanticDiffEntry{Path: childPath, Kind: SemanticDiffAdded}); err != nil {
					return err
				}
			case !afterExists:
				if err := appendSemanticDiff(changes, SemanticDiffEntry{Path: childPath, Kind: SemanticDiffRemoved}); err != nil {
					return err
				}
			default:
				if err := collectSemanticDiff(childPath, beforeValue, afterValue, changes); err != nil {
					return err
				}
			}
		}
		return nil
	}

	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("%w: encode base value at %s", ErrInvalidSemanticDiff, path)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("%w: encode target value at %s", ErrInvalidSemanticDiff, path)
	}
	if bytes.Equal(beforeJSON, afterJSON) {
		return nil
	}
	if path == "" {
		path = "/"
	}
	return appendSemanticDiff(changes, SemanticDiffEntry{Path: path, Kind: SemanticDiffChanged})
}

func appendSemanticDiff(changes *[]SemanticDiffEntry, entry SemanticDiffEntry) error {
	if entry.Path == "" || !strings.HasPrefix(entry.Path, "/") || len(entry.Path) > MaxSemanticDiffPathLength {
		return fmt.Errorf("%w: invalid or overlong diff path", ErrInvalidSemanticDiff)
	}
	if len(*changes) >= MaxSemanticDiffEntries {
		return fmt.Errorf("%w: diff exceeds %d entries", ErrInvalidSemanticDiff, MaxSemanticDiffEntries)
	}
	*changes = append(*changes, entry)
	return nil
}

func escapeJSONPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}
