package reporting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxFormulaBytes  = 1024
	maxFormulaTokens = 128
	maxFormulaDepth  = 16
)

// Formula is a compiled, side-effect-free arithmetic expression over row fields.
// It deliberately supports only numeric literals, field references, parentheses,
// and +, -, * and / operators. There are no functions, strings, environment
// lookups or dynamic calls, so report formulas cannot become an execution path.
type Formula struct {
	source string
	root   expression
}

// CompileFormula validates and compiles a report formula. Compilation is bounded
// to keep user-supplied report definitions from consuming unbounded CPU or stack.
func CompileFormula(source string) (*Formula, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, errors.New("formula is required")
	}
	if len(source) > maxFormulaBytes {
		return nil, fmt.Errorf("formula exceeds %d bytes", maxFormulaBytes)
	}

	p := formulaParser{input: source}
	root, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(p.input) {
		return nil, fmt.Errorf("unexpected token at byte %d", p.pos)
	}
	return &Formula{source: source, root: root}, nil
}

// Source returns the normalized source expression used to compile the formula.
func (f *Formula) Source() string {
	if f == nil {
		return ""
	}
	return f.source
}

// Evaluate computes the formula for a row. Missing, non-numeric and non-finite
// values fail closed rather than silently becoming zero.
func (f *Formula) Evaluate(row Row) (float64, error) {
	if f == nil || f.root == nil {
		return 0, errors.New("formula is not compiled")
	}
	value, err := f.root.eval(row)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("formula result is not finite")
	}
	return value, nil
}

type expression interface {
	eval(Row) (float64, error)
}

type numberExpression float64

func (e numberExpression) eval(Row) (float64, error) { return float64(e), nil }

type fieldExpression string

func (e fieldExpression) eval(row Row) (float64, error) {
	name := string(e)
	value, ok := row[name]
	if !ok {
		return 0, fmt.Errorf("formula field %q is missing", name)
	}
	return numericValue(name, value)
}

type unaryExpression struct {
	op    byte
	value expression
}

func (e unaryExpression) eval(row Row) (float64, error) {
	value, err := e.value.eval(row)
	if err != nil {
		return 0, err
	}
	if e.op == '-' {
		return -value, nil
	}
	return value, nil
}

type binaryExpression struct {
	op          byte
	left, right expression
}

func (e binaryExpression) eval(row Row) (float64, error) {
	left, err := e.left.eval(row)
	if err != nil {
		return 0, err
	}
	right, err := e.right.eval(row)
	if err != nil {
		return 0, err
	}

	var result float64
	switch e.op {
	case '+':
		result = left + right
	case '-':
		result = left - right
	case '*':
		result = left * right
	case '/':
		if right == 0 {
			return 0, errors.New("division by zero")
		}
		result = left / right
	default:
		return 0, errors.New("unsupported formula operator")
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, errors.New("formula operation produced a non-finite value")
	}
	return result, nil
}

type formulaParser struct {
	input  string
	pos    int
	tokens int
}

func (p *formulaParser) parseExpression(depth int) (expression, error) {
	if depth > maxFormulaDepth {
		return nil, fmt.Errorf("formula nesting exceeds %d", maxFormulaDepth)
	}
	left, err := p.parseTerm(depth)
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if !p.peek('+') && !p.peek('-') {
			return left, nil
		}
		op := p.input[p.pos]
		p.pos++
		if err := p.countToken(); err != nil {
			return nil, err
		}
		right, err := p.parseTerm(depth)
		if err != nil {
			return nil, err
		}
		left = binaryExpression{op: op, left: left, right: right}
	}
}

func (p *formulaParser) parseTerm(depth int) (expression, error) {
	left, err := p.parseUnary(depth)
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if !p.peek('*') && !p.peek('/') {
			return left, nil
		}
		op := p.input[p.pos]
		p.pos++
		if err := p.countToken(); err != nil {
			return nil, err
		}
		right, err := p.parseUnary(depth)
		if err != nil {
			return nil, err
		}
		left = binaryExpression{op: op, left: left, right: right}
	}
}

func (p *formulaParser) parseUnary(depth int) (expression, error) {
	p.skipSpace()
	if p.peek('+') || p.peek('-') {
		op := p.input[p.pos]
		p.pos++
		if err := p.countToken(); err != nil {
			return nil, err
		}
		value, err := p.parseUnary(depth)
		if err != nil {
			return nil, err
		}
		return unaryExpression{op: op, value: value}, nil
	}
	return p.parsePrimary(depth)
}

func (p *formulaParser) parsePrimary(depth int) (expression, error) {
	p.skipSpace()
	if p.pos >= len(p.input) {
		return nil, errors.New("unexpected end of formula")
	}
	if p.peek('(') {
		p.pos++
		if err := p.countToken(); err != nil {
			return nil, err
		}
		value, err := p.parseExpression(depth + 1)
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if !p.peek(')') {
			return nil, fmt.Errorf("missing closing parenthesis at byte %d", p.pos)
		}
		p.pos++
		if err := p.countToken(); err != nil {
			return nil, err
		}
		return value, nil
	}

	if isIdentifierStart(rune(p.input[p.pos])) {
		start := p.pos
		p.pos++
		for p.pos < len(p.input) && isIdentifierContinue(rune(p.input[p.pos])) {
			p.pos++
		}
		if err := p.countToken(); err != nil {
			return nil, err
		}
		return fieldExpression(p.input[start:p.pos]), nil
	}

	start := p.pos
	seenDigit := false
	seenDot := false
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
			p.pos++
		case c == '.' && !seenDot:
			seenDot = true
			p.pos++
		default:
			goto parsedNumber
		}
	}
parsedNumber:
	if !seenDigit {
		return nil, fmt.Errorf("invalid token at byte %d", start)
	}
	value, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("invalid number at byte %d", start)
	}
	if err := p.countToken(); err != nil {
		return nil, err
	}
	return numberExpression(value), nil
}

func (p *formulaParser) countToken() error {
	p.tokens++
	if p.tokens > maxFormulaTokens {
		return fmt.Errorf("formula exceeds %d tokens", maxFormulaTokens)
	}
	return nil
}

func (p *formulaParser) skipSpace() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *formulaParser) peek(value byte) bool {
	return p.pos < len(p.input) && p.input[p.pos] == value
}

func isIdentifierStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentifierContinue(r rune) bool {
	return isIdentifierStart(r) || unicode.IsDigit(r) || r == '.'
}

func numericValue(field string, value any) (float64, error) {
	var result float64
	switch v := value.(type) {
	case int:
		result = float64(v)
	case int8:
		result = float64(v)
	case int16:
		result = float64(v)
	case int32:
		result = float64(v)
	case int64:
		result = float64(v)
	case uint:
		result = float64(v)
	case uint8:
		result = float64(v)
	case uint16:
		result = float64(v)
	case uint32:
		result = float64(v)
	case uint64:
		result = float64(v)
	case float32:
		result = float64(v)
	case float64:
		result = v
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("formula field %q is not numeric: %w", field, err)
		}
		result = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, fmt.Errorf("formula field %q is not numeric", field)
		}
		result = parsed
	default:
		return 0, fmt.Errorf("formula field %q has unsupported numeric type %T", field, value)
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, fmt.Errorf("formula field %q is not finite", field)
	}
	return result, nil
}
