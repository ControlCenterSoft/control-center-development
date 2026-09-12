package reporting

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultMaxRows        = 10000
	defaultMaxColumns     = 128
	defaultMaxCellBytes   = 64 * 1024
	defaultMaxOutputBytes = 64 * 1024 * 1024
)

// Row is one reporting record. Formula fields are resolved by exact key.
type Row map[string]any

// Column describes either a direct field projection or a compiled formula.
// Exactly one of Field or Formula must be set.
type Column struct {
	Header  string
	Field   string
	Formula *Formula
}

// ExportLimits bounds report materialization before any bytes are returned.
type ExportLimits struct {
	MaxRows        int
	MaxColumns     int
	MaxCellBytes   int
	MaxOutputBytes int
}

func DefaultExportLimits() ExportLimits {
	return ExportLimits{
		MaxRows:        defaultMaxRows,
		MaxColumns:     defaultMaxColumns,
		MaxCellBytes:   defaultMaxCellBytes,
		MaxOutputBytes: defaultMaxOutputBytes,
	}
}

// ExportCSV renders deterministic UTF-8 CSV. String cells that spreadsheet
// applications could interpret as formulas are prefixed with an apostrophe to
// prevent CSV/formula injection when an administrator opens an export locally.
func ExportCSV(columns []Column, rows []Row, limits ExportLimits) ([]byte, error) {
	limits = normalizedLimits(limits)
	if len(columns) == 0 {
		return nil, errors.New("at least one report column is required")
	}
	if len(columns) > limits.MaxColumns {
		return nil, fmt.Errorf("report has %d columns; limit is %d", len(columns), limits.MaxColumns)
	}
	if len(rows) > limits.MaxRows {
		return nil, fmt.Errorf("report has %d rows; limit is %d", len(rows), limits.MaxRows)
	}

	headers := make([]string, len(columns))
	seenHeaders := make(map[string]struct{}, len(columns))
	for i, column := range columns {
		header := strings.TrimSpace(column.Header)
		if header == "" {
			return nil, fmt.Errorf("column %d has an empty header", i)
		}
		if !utf8.ValidString(header) {
			return nil, fmt.Errorf("column %q header is not valid UTF-8", header)
		}
		if _, duplicate := seenHeaders[header]; duplicate {
			return nil, fmt.Errorf("duplicate column header %q", header)
		}
		seenHeaders[header] = struct{}{}
		if err := validateColumn(column); err != nil {
			return nil, fmt.Errorf("column %q: %w", header, err)
		}
		header = protectSpreadsheetFormula(header)
		if len(header) > limits.MaxCellBytes {
			return nil, fmt.Errorf("column %q header exceeds %d bytes", column.Header, limits.MaxCellBytes)
		}
		headers[i] = header
	}

	buffer := cappedBuffer{maxBytes: limits.MaxOutputBytes}
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("write CSV header: %w", err)
	}

	for rowIndex, row := range rows {
		record := make([]string, len(columns))
		for columnIndex, column := range columns {
			cell, err := renderCell(column, row)
			if err != nil {
				return nil, fmt.Errorf("row %d column %q: %w", rowIndex+1, column.Header, err)
			}
			if !utf8.ValidString(cell) {
				return nil, fmt.Errorf("row %d column %q is not valid UTF-8", rowIndex+1, column.Header)
			}
			if len(cell) > limits.MaxCellBytes {
				return nil, fmt.Errorf("row %d column %q exceeds %d bytes", rowIndex+1, column.Header, limits.MaxCellBytes)
			}
			record[columnIndex] = cell
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("write CSV row %d: %w", rowIndex+1, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush CSV: %w", err)
	}
	return append([]byte(nil), buffer.Bytes()...), nil
}

func normalizedLimits(limits ExportLimits) ExportLimits {
	defaults := DefaultExportLimits()
	if limits.MaxRows <= 0 {
		limits.MaxRows = defaults.MaxRows
	}
	if limits.MaxColumns <= 0 {
		limits.MaxColumns = defaults.MaxColumns
	}
	if limits.MaxCellBytes <= 0 {
		limits.MaxCellBytes = defaults.MaxCellBytes
	}
	if limits.MaxOutputBytes <= 0 {
		limits.MaxOutputBytes = defaults.MaxOutputBytes
	}
	return limits
}

func validateColumn(column Column) error {
	field := strings.TrimSpace(column.Field)
	hasField := field != ""
	hasFormula := column.Formula != nil
	if hasField == hasFormula {
		return errors.New("exactly one of field or formula is required")
	}
	if hasField && field != column.Field {
		return errors.New("field name must not contain leading or trailing whitespace")
	}
	return nil
}

func renderCell(column Column, row Row) (string, error) {
	if column.Formula != nil {
		value, err := column.Formula.Evaluate(row)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat(value, 'g', -1, 64), nil
	}
	value, exists := row[column.Field]
	if !exists || value == nil {
		return "", nil
	}
	return scalarCell(value)
}

func scalarCell(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return protectSpreadsheetFormula(v), nil
	case []byte:
		return protectSpreadsheetFormula(string(v)), nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.FormatInt(int64(v), 10), nil
	case int8:
		return strconv.FormatInt(int64(v), 10), nil
	case int16:
		return strconv.FormatInt(int64(v), 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case float32:
		return finiteFloat(float64(v), 32)
	case float64:
		return finiteFloat(v, 64)
	case json.Number:
		parsed, err := v.Float64()
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return "", errors.New("JSON number is not finite")
		}
		return v.String(), nil
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano), nil
	default:
		return "", fmt.Errorf("unsupported scalar type %T", value)
	}
}

func finiteFloat(value float64, bitSize int) (string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", errors.New("floating-point value is not finite")
	}
	return strconv.FormatFloat(value, 'g', -1, bitSize), nil
}

func protectSpreadsheetFormula(value string) string {
	if value == "" {
		return value
	}
	candidate := strings.TrimLeft(value, " ")
	if candidate == "" {
		return value
	}
	switch candidate[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + value
	default:
		return value
	}
}

type cappedBuffer struct {
	bytes.Buffer
	maxBytes int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.maxBytes <= 0 || b.Len()+len(p) > b.maxBytes {
		return 0, fmt.Errorf("CSV output exceeds %d bytes", b.maxBytes)
	}
	return b.Buffer.Write(p)
}
