package postgres

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const maxAuditJSONBCanonicalBytes = 16 * 1024

// validateAuditJSONBCanonicalSize rejects compact JSON numbers whose PostgreSQL
// jsonb decimal rendering could expand the audit details beyond the existing
// 16 KiB persistence/hash budget. It performs bounded arithmetic only and does
// not materialize the expanded decimal representation.
func validateAuditJSONBCanonicalSize(details map[string]any, compactBytes int) error {
	growth, spacing, err := estimateJSONBCanonicalOverhead(details)
	if err != nil {
		return err
	}
	if compactBytes > maxAuditJSONBCanonicalBytes-growth-spacing {
		return fmt.Errorf("audit details PostgreSQL jsonb representation exceeds %d-byte limit", maxAuditJSONBCanonicalBytes)
	}
	return nil
}

func estimateJSONBCanonicalOverhead(value any) (growth int, spacing int, err error) {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) > 0 {
			spacing += len(typed)     // jsonb prints one space after every colon
			spacing += len(typed) - 1 // and one after each separating comma
		}
		for _, child := range typed {
			childGrowth, childSpacing, childErr := estimateJSONBCanonicalOverhead(child)
			if childErr != nil {
				return 0, 0, childErr
			}
			growth, err = boundedAdd(growth, childGrowth)
			if err != nil {
				return 0, 0, err
			}
			spacing, err = boundedAdd(spacing, childSpacing)
			if err != nil {
				return 0, 0, err
			}
		}
	case []any:
		if len(typed) > 0 {
			spacing += len(typed) - 1
		}
		for _, child := range typed {
			childGrowth, childSpacing, childErr := estimateJSONBCanonicalOverhead(child)
			if childErr != nil {
				return 0, 0, childErr
			}
			growth, err = boundedAdd(growth, childGrowth)
			if err != nil {
				return 0, 0, err
			}
			spacing, err = boundedAdd(spacing, childSpacing)
			if err != nil {
				return 0, 0, err
			}
		}
	case json.Number:
		expanded, numberErr := estimateExpandedJSONNumberBytes(typed.String())
		if numberErr != nil {
			return 0, 0, fmt.Errorf("invalid audit JSON number %q: %w", typed.String(), numberErr)
		}
		if expanded > len(typed.String()) {
			growth = expanded - len(typed.String())
		}
	}
	if growth+spacing > maxAuditJSONBCanonicalBytes {
		return 0, 0, fmt.Errorf("audit details PostgreSQL jsonb representation exceeds %d-byte limit", maxAuditJSONBCanonicalBytes)
	}
	return growth, spacing, nil
}

func boundedAdd(current, delta int) (int, error) {
	if delta > maxAuditJSONBCanonicalBytes-current {
		return 0, fmt.Errorf("audit details PostgreSQL jsonb representation exceeds %d-byte limit", maxAuditJSONBCanonicalBytes)
	}
	return current + delta, nil
}

func estimateExpandedJSONNumberBytes(number string) (int, error) {
	if number == "" {
		return 0, fmt.Errorf("empty number")
	}
	start := 0
	signBytes := 0
	if number[0] == '-' {
		signBytes = 1
		start = 1
		if start == len(number) {
			return 0, fmt.Errorf("missing digits")
		}
	}

	exponentIndex := strings.IndexAny(number[start:], "eE")
	mantissaEnd := len(number)
	exponent := 0
	if exponentIndex >= 0 {
		exponentIndex += start
		mantissaEnd = exponentIndex
		parsed, err := parseBoundedExponent(number[exponentIndex+1:])
		if err != nil {
			return 0, err
		}
		exponent = parsed
	}
	mantissa := number[start:mantissaEnd]
	if mantissa == "" {
		return 0, fmt.Errorf("missing mantissa")
	}
	dot := strings.IndexByte(mantissa, '.')
	integerDigits := len(mantissa)
	totalDigits := len(mantissa)
	if dot >= 0 {
		integerDigits = dot
		totalDigits--
	}
	if integerDigits <= 0 || totalDigits <= 0 {
		return 0, fmt.Errorf("invalid mantissa")
	}

	decimalPosition := integerDigits + exponent
	var expanded int
	switch {
	case decimalPosition <= 0:
		expanded = signBytes + 2 + (-decimalPosition) + totalDigits // 0.<zeros><digits>
	case decimalPosition >= totalDigits:
		expanded = signBytes + decimalPosition // digits plus trailing zeros
	default:
		expanded = signBytes + totalDigits + 1 // embedded decimal point
	}
	if expanded > maxAuditJSONBCanonicalBytes {
		return expanded, nil
	}
	return expanded, nil
}

func parseBoundedExponent(raw string) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("missing exponent")
	}
	sign := 1
	if raw[0] == '+' || raw[0] == '-' {
		if raw[0] == '-' {
			sign = -1
		}
		raw = raw[1:]
		if raw == "" {
			return 0, fmt.Errorf("missing exponent digits")
		}
	}
	if len(raw) > 6 {
		return sign * (maxAuditJSONBCanonicalBytes + 1), nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid exponent")
	}
	if value > maxAuditJSONBCanonicalBytes {
		value = maxAuditJSONBCanonicalBytes + 1
	}
	return sign * value, nil
}
