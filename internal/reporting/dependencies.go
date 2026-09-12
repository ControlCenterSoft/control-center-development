package reporting

import (
	"fmt"
	"sort"
	"strings"
)

// Fields returns the unique row fields referenced by the formula in stable
// lexical order. Callers can use this before querying data to authorize every
// source field that a formula would read.
func (f *Formula) Fields() []string {
	if f == nil || f.root == nil {
		return nil
	}
	fields := make(map[string]struct{})
	collectFormulaFields(f.root, fields)
	result := make([]string, 0, len(fields))
	for field := range fields {
		result = append(result, field)
	}
	sort.Strings(result)
	return result
}

// RequiredFields returns every direct or formula-dependent source field needed
// to render the report. The result is intended for pre-query RBAC/tenant-scope
// authorization; it never grants access on its own.
func RequiredFields(columns []Column) ([]string, error) {
	fields := make(map[string]struct{})
	for index, column := range columns {
		if err := validateColumn(column); err != nil {
			return nil, fmt.Errorf("column %d: %w", index, err)
		}
		if field := strings.TrimSpace(column.Field); field != "" {
			fields[field] = struct{}{}
		}
		if column.Formula != nil {
			for _, field := range column.Formula.Fields() {
				fields[field] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(fields))
	for field := range fields {
		result = append(result, field)
	}
	sort.Strings(result)
	return result, nil
}

func collectFormulaFields(expr expression, fields map[string]struct{}) {
	switch value := expr.(type) {
	case fieldExpression:
		fields[string(value)] = struct{}{}
	case unaryExpression:
		collectFormulaFields(value.value, fields)
	case binaryExpression:
		collectFormulaFields(value.left, fields)
		collectFormulaFields(value.right, fields)
	}
}
