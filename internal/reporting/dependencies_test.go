package reporting

import (
	"reflect"
	"testing"
)

func TestFormulaFieldsAreUniqueAndSorted(t *testing.T) {
	formula, err := CompileFormula("used / total + used + nested.value")
	if err != nil {
		t.Fatalf("compile formula: %v", err)
	}
	want := []string{"nested.value", "total", "used"}
	if got := formula.Fields(); !reflect.DeepEqual(got, want) {
		t.Fatalf("fields: got %#v want %#v", got, want)
	}
}

func TestRequiredFieldsIncludesDirectAndFormulaDependencies(t *testing.T) {
	formula, err := CompileFormula("used / total * 100")
	if err != nil {
		t.Fatalf("compile formula: %v", err)
	}
	columns := []Column{
		{Header: "Node", Field: "node"},
		{Header: "Utilization", Formula: formula},
		{Header: "Owner", Field: "owner"},
	}
	want := []string{"node", "owner", "total", "used"}
	got, err := RequiredFields(columns)
	if err != nil {
		t.Fatalf("required fields: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields: got %#v want %#v", got, want)
	}
}

func TestRequiredFieldsRejectsInvalidColumns(t *testing.T) {
	if _, err := RequiredFields([]Column{{Header: "Broken"}}); err == nil {
		t.Fatal("expected invalid column to fail closed")
	}
}
