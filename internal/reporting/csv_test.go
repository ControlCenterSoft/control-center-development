package reporting

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestExportCSVDirectAndFormulaColumns(t *testing.T) {
	utilization, err := CompileFormula("used / total * 100")
	if err != nil {
		t.Fatalf("compile formula: %v", err)
	}

	data, err := ExportCSV([]Column{
		{Header: "Node", Field: "node"},
		{Header: "Utilization", Formula: utilization},
		{Header: "Observed", Field: "observed_at"},
	}, []Row{{
		"node":        "node-01",
		"used":        3,
		"total":       4,
		"observed_at": time.Date(2026, 9, 12, 12, 0, 0, 123, time.FixedZone("test", 3*60*60)),
	}}, DefaultExportLimits())
	if err != nil {
		t.Fatalf("export CSV: %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatalf("read exported CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("unexpected record count: %d", len(records))
	}
	want := []string{"node-01", "75", "2026-09-12T09:00:00.000000123Z"}
	for i := range want {
		if records[1][i] != want[i] {
			t.Fatalf("cell %d: got %q want %q", i, records[1][i], want[i])
		}
	}
}

func TestExportCSVProtectsSpreadsheetFormulaInjection(t *testing.T) {
	rows := []Row{
		{"value": "=HYPERLINK(\"https://example.invalid\")"},
		{"value": "  +1+1"},
		{"value": "-10"},
		{"value": "@SUM(A1:A2)"},
		{"value": "\tcmd"},
		{"value": "  \rpayload"},
		{"value": 42},
		{"value": -10},
	}
	data, err := ExportCSV([]Column{{Header: "Value", Field: "value"}}, rows, DefaultExportLimits())
	if err != nil {
		t.Fatalf("export CSV: %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatalf("read exported CSV: %v", err)
	}
	want := []string{
		"'=HYPERLINK(\"https://example.invalid\")",
		"'  +1+1",
		"'-10",
		"'@SUM(A1:A2)",
		"'\tcmd",
		"'  \rpayload",
		"42",
		"-10",
	}
	for i, expected := range want {
		if got := records[i+1][0]; got != expected {
			t.Fatalf("row %d: got %q want %q", i+1, got, expected)
		}
	}
}

func TestExportCSVValidatesSchemaAndLimits(t *testing.T) {
	formula, err := CompileFormula("a + b")
	if err != nil {
		t.Fatalf("compile formula: %v", err)
	}

	cases := []struct {
		name    string
		columns []Column
		rows    []Row
		limits  ExportLimits
	}{
		{name: "no columns", limits: DefaultExportLimits()},
		{name: "duplicate headers", columns: []Column{{Header: "A", Field: "a"}, {Header: "A", Field: "b"}}, limits: DefaultExportLimits()},
		{name: "field and formula", columns: []Column{{Header: "A", Field: "a", Formula: formula}}, limits: DefaultExportLimits()},
		{name: "neither field nor formula", columns: []Column{{Header: "A"}}, limits: DefaultExportLimits()},
		{name: "row limit", columns: []Column{{Header: "A", Field: "a"}}, rows: []Row{{"a": 1}, {"a": 2}}, limits: ExportLimits{MaxRows: 1, MaxColumns: 4, MaxCellBytes: 128}},
		{name: "column limit", columns: []Column{{Header: "A", Field: "a"}, {Header: "B", Field: "b"}}, limits: ExportLimits{MaxRows: 4, MaxColumns: 1, MaxCellBytes: 128}},
		{name: "cell limit", columns: []Column{{Header: "A", Field: "a"}}, rows: []Row{{"a": "too long"}}, limits: ExportLimits{MaxRows: 4, MaxColumns: 4, MaxCellBytes: 3}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ExportCSV(tc.columns, tc.rows, tc.limits); err == nil {
				t.Fatal("expected export validation error")
			}
		})
	}
}

func TestExportCSVRejectsUnsupportedScalarType(t *testing.T) {
	_, err := ExportCSV(
		[]Column{{Header: "Value", Field: "value"}},
		[]Row{{"value": map[string]string{"secret": "not a scalar"}}},
		DefaultExportLimits(),
	)
	if err == nil {
		t.Fatal("expected unsupported scalar type to fail closed")
	}
}
