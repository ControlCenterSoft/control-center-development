package reporting

import "testing"

func TestExportCSVEnforcesTotalOutputLimit(t *testing.T) {
	_, err := ExportCSV(
		[]Column{{Header: "Value", Field: "value"}},
		[]Row{{"value": "1234567890"}, {"value": "abcdefghij"}},
		ExportLimits{
			MaxRows:        10,
			MaxColumns:     10,
			MaxCellBytes:   32,
			MaxOutputBytes: 12,
		},
	)
	if err == nil {
		t.Fatal("expected total CSV output limit to fail closed")
	}
}
