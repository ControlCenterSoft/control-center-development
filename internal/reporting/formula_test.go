package reporting

import (
	"math"
	"strings"
	"testing"
)

func TestFormulaPrecedenceAndFields(t *testing.T) {
	formula, err := CompileFormula("used_cpu / total_cpu * 100 + adjustment")
	if err != nil {
		t.Fatalf("compile formula: %v", err)
	}
	got, err := formula.Evaluate(Row{
		"used_cpu":   6,
		"total_cpu":  8,
		"adjustment": "2.5",
	})
	if err != nil {
		t.Fatalf("evaluate formula: %v", err)
	}
	if got != 77.5 {
		t.Fatalf("unexpected result: got %v want 77.5", got)
	}
}

func TestFormulaParenthesesAndUnary(t *testing.T) {
	formula, err := CompileFormula("-(free - reserved) / 2")
	if err != nil {
		t.Fatalf("compile formula: %v", err)
	}
	got, err := formula.Evaluate(Row{"free": 12, "reserved": 4})
	if err != nil {
		t.Fatalf("evaluate formula: %v", err)
	}
	if got != -4 {
		t.Fatalf("unexpected result: got %v want -4", got)
	}
}

func TestFormulaFailsClosedOnMissingAndInvalidData(t *testing.T) {
	formula, err := CompileFormula("used / total")
	if err != nil {
		t.Fatalf("compile formula: %v", err)
	}

	cases := []struct {
		name string
		row  Row
	}{
		{name: "missing", row: Row{"used": 1}},
		{name: "division by zero", row: Row{"used": 1, "total": 0}},
		{name: "non numeric", row: Row{"used": "many", "total": 2}},
		{name: "non finite", row: Row{"used": math.Inf(1), "total": 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := formula.Evaluate(tc.row); err == nil {
				t.Fatal("expected fail-closed evaluation error")
			}
		})
	}
}

func TestFormulaRejectsUnsupportedSyntax(t *testing.T) {
	for _, source := range []string{
		"",
		"system('id')",
		"a ** b",
		"a +",
		"a ? b : c",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := CompileFormula(source); err == nil {
				t.Fatalf("expected %q to be rejected", source)
			}
		})
	}
}

func TestFormulaBoundsComplexity(t *testing.T) {
	tooLong := strings.Repeat("1+", maxFormulaBytes/2) + "1"
	if _, err := CompileFormula(tooLong); err == nil {
		t.Fatal("expected oversized formula to be rejected")
	}

	deep := strings.Repeat("(", maxFormulaDepth+2) + "1" + strings.Repeat(")", maxFormulaDepth+2)
	if _, err := CompileFormula(deep); err == nil {
		t.Fatal("expected deeply nested formula to be rejected")
	}

	manyTokens := strings.Repeat("1+", maxFormulaTokens) + "1"
	if _, err := CompileFormula(manyTokens); err == nil {
		t.Fatal("expected token-heavy formula to be rejected")
	}
}
