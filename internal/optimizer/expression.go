package optimizer

import (
	"fmt"
	"math"
	"strconv"
	"unicode"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/platform/apperrors"
)

type Expression struct {
	root expressionNode
}

type expressionNode interface {
	evaluate(vars map[string]float64) (float64, error)
}

type numberNode float64

func (n numberNode) evaluate(_ map[string]float64) (float64, error) {
	return float64(n), nil
}

type variableNode string

func (n variableNode) evaluate(vars map[string]float64) (float64, error) {
	value, ok := vars[string(n)]
	if !ok {
		return 0, validationError("missing variable %q", string(n))
	}
	return value, nil
}

type unaryNode struct {
	op   tokenType
	node expressionNode
}

func (n unaryNode) evaluate(vars map[string]float64) (float64, error) {
	value, err := n.node.evaluate(vars)
	if err != nil {
		return 0, err
	}
	if n.op == tokenMinus {
		return -value, nil
	}
	return value, nil
}

type binaryNode struct {
	op          tokenType
	left, right expressionNode
}

func (n binaryNode) evaluate(vars map[string]float64) (float64, error) {
	left, err := n.left.evaluate(vars)
	if err != nil {
		return 0, err
	}
	right, err := n.right.evaluate(vars)
	if err != nil {
		return 0, err
	}
	switch n.op {
	case tokenPlus:
		return left + right, nil
	case tokenMinus:
		return left - right, nil
	case tokenStar:
		return left * right, nil
	case tokenSlash:
		if right == 0 {
			return 0, validationError("division by zero")
		}
		return left / right, nil
	default:
		return 0, validationError("unsupported operator")
	}
}

func CompileExpression(input string) (Expression, error) {
	return compileExpression(input, defaultAllowedVariables())
}

func compileExpression(input string, allowed map[string]struct{}) (Expression, error) {
	tokens, err := lexExpression(input)
	if err != nil {
		return Expression{}, err
	}
	parser := expressionParser{tokens: tokens, allowed: allowed}
	root, err := parser.parseExpression()
	if err != nil {
		return Expression{}, err
	}
	if parser.current().typ != tokenEOF {
		return Expression{}, validationError("unexpected token %q", parser.current().literal)
	}
	return Expression{root: root}, nil
}

func (e Expression) Evaluate(vars map[string]float64) (float64, error) {
	value, err := e.root.evaluate(vars)
	if err != nil {
		return 0, err
	}
	value = roundFloat(value)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, validationError("expression result must be finite")
	}
	return value, nil
}

func defaultAllowedVariables() map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, name := range eval.DefaultMetricRegistry.Names() {
		allowed[name] = struct{}{}
	}
	allowed["normalized_latency"] = struct{}{}
	allowed["normalized_cost"] = struct{}{}
	allowed["pairwise_win_rate"] = struct{}{}
	return allowed
}

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenNumber
	tokenIdentifier
	tokenPlus
	tokenMinus
	tokenStar
	tokenSlash
	tokenLeftParen
	tokenRightParen
	tokenComma
)

type token struct {
	typ     tokenType
	literal string
}

func lexExpression(input string) ([]token, error) {
	var tokens []token
	for i := 0; i < len(input); {
		r := rune(input[i])
		switch {
		case unicode.IsSpace(r):
			i++
		case isIdentStart(r):
			start := i
			i++
			for i < len(input) && isIdentPart(rune(input[i])) {
				i++
			}
			tokens = append(tokens, token{typ: tokenIdentifier, literal: input[start:i]})
		case unicode.IsDigit(r) || r == '.':
			start := i
			dotSeen := r == '.'
			i++
			for i < len(input) {
				next := rune(input[i])
				if next == '.' {
					if dotSeen {
						return nil, validationError("invalid number %q", input[start:i+1])
					}
					dotSeen = true
					i++
					continue
				}
				if !unicode.IsDigit(next) {
					break
				}
				i++
			}
			literal := input[start:i]
			if literal == "." {
				return nil, validationError("invalid character %q", ".")
			}
			if _, err := strconv.ParseFloat(literal, 64); err != nil {
				return nil, validationError("invalid number %q", literal)
			}
			tokens = append(tokens, token{typ: tokenNumber, literal: literal})
		case r == '+':
			tokens = append(tokens, token{typ: tokenPlus, literal: "+"})
			i++
		case r == '-':
			tokens = append(tokens, token{typ: tokenMinus, literal: "-"})
			i++
		case r == '*':
			tokens = append(tokens, token{typ: tokenStar, literal: "*"})
			i++
		case r == '/':
			tokens = append(tokens, token{typ: tokenSlash, literal: "/"})
			i++
		case r == '(':
			tokens = append(tokens, token{typ: tokenLeftParen, literal: "("})
			i++
		case r == ')':
			tokens = append(tokens, token{typ: tokenRightParen, literal: ")"})
			i++
		case r == ',':
			tokens = append(tokens, token{typ: tokenComma, literal: ","})
			i++
		default:
			return nil, validationError("invalid character %q", string(r))
		}
	}
	tokens = append(tokens, token{typ: tokenEOF})
	return tokens, nil
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || unicode.IsDigit(r)
}

type expressionParser struct {
	tokens  []token
	pos     int
	allowed map[string]struct{}
}

func (p *expressionParser) parseExpression() (expressionNode, error) {
	return p.parseAdditive()
}

func (p *expressionParser) parseAdditive() (expressionNode, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.current().typ == tokenPlus || p.current().typ == tokenMinus {
		op := p.advance().typ
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
	return left, nil
}

func (p *expressionParser) parseMultiplicative() (expressionNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.current().typ == tokenStar || p.current().typ == tokenSlash {
		op := p.advance().typ
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
	return left, nil
}

func (p *expressionParser) parseUnary() (expressionNode, error) {
	if p.current().typ == tokenPlus || p.current().typ == tokenMinus {
		op := p.advance().typ
		node, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: op, node: node}, nil
	}
	return p.parsePrimary()
}

func (p *expressionParser) parsePrimary() (expressionNode, error) {
	tok := p.current()
	switch tok.typ {
	case tokenNumber:
		p.advance()
		value, _ := strconv.ParseFloat(tok.literal, 64)
		return numberNode(value), nil
	case tokenIdentifier:
		p.advance()
		if p.current().typ == tokenLeftParen {
			return nil, validationError("function calls are not allowed")
		}
		if _, ok := p.allowed[tok.literal]; !ok {
			return nil, validationError("unknown variable %q", tok.literal)
		}
		return variableNode(tok.literal), nil
	case tokenLeftParen:
		p.advance()
		node, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if p.current().typ != tokenRightParen {
			return nil, validationError("missing closing parenthesis")
		}
		p.advance()
		return node, nil
	default:
		return nil, validationError("unexpected token %q", tok.literal)
	}
}

func (p *expressionParser) current() token {
	return p.tokens[p.pos]
}

func (p *expressionParser) advance() token {
	tok := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return tok
}

func validationError(format string, args ...any) error {
	return apperrors.New(apperrors.CodeValidation, fmt.Sprintf(format, args...))
}

func roundFloat(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	const scale = 1e12
	if math.Abs(value) > math.MaxFloat64/scale {
		return value
	}
	return math.Round(value*scale) / scale
}
