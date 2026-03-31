package expr

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/event"
)

// Expression can be evaluated against an event.
type Expression interface {
	Evaluate(e *event.Event) bool
}

// Parse parses an expression string into an evaluatable expression.
func Parse(input string) (Expression, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().typ != tokenEOF {
		return nil, fmt.Errorf("unexpected token %q at position %d", p.peek().value, p.peek().pos)
	}
	return expr, nil
}

// --- Token types ---

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenLParen
	tokenRParen
	tokenAnd
	tokenOr
	tokenEq
	tokenNeq
	tokenString
	tokenBool
	tokenNull
	tokenIdent
	tokenHas
	tokenComma
)

type token struct {
	typ   tokenType
	value string
	pos   int
}

// --- Tokenizer ---

func tokenize(input string) ([]token, error) {
	var tokens []token
	i := 0

	for i < len(input) {
		// Skip whitespace
		if unicode.IsSpace(rune(input[i])) {
			i++
			continue
		}

		switch input[i] {
		case '(':
			tokens = append(tokens, token{tokenLParen, "(", i})
			i++
		case ')':
			tokens = append(tokens, token{tokenRParen, ")", i})
			i++
		case ',':
			tokens = append(tokens, token{tokenComma, ",", i})
			i++
		case '=':
			tokens = append(tokens, token{tokenEq, "=", i})
			i++
		case '!':
			if i+1 < len(input) && input[i+1] == '=' {
				tokens = append(tokens, token{tokenNeq, "!=", i})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected character '!' at position %d", i)
			}
		case '"':
			start := i
			i++ // skip opening quote
			var sb strings.Builder
			for i < len(input) && input[i] != '"' {
				if input[i] == '\\' && i+1 < len(input) {
					i++
					switch input[i] {
					case '"', '\\':
						sb.WriteByte(input[i])
					default:
						sb.WriteByte('\\')
						sb.WriteByte(input[i])
					}
				} else {
					sb.WriteByte(input[i])
				}
				i++
			}
			if i >= len(input) {
				return nil, fmt.Errorf("unterminated string at position %d", start)
			}
			i++ // skip closing quote
			tokens = append(tokens, token{tokenString, sb.String(), start})
		default:
			if isIdentStart(input[i]) {
				start := i
				for i < len(input) && isIdentPart(input[i]) {
					i++
				}
				word := input[start:i]
				switch word {
				case "AND":
					tokens = append(tokens, token{tokenAnd, word, start})
				case "OR":
					tokens = append(tokens, token{tokenOr, word, start})
				case "true", "false":
					tokens = append(tokens, token{tokenBool, word, start})
				case "null":
					tokens = append(tokens, token{tokenNull, word, start})
				case "has":
					tokens = append(tokens, token{tokenHas, word, start})
				default:
					tokens = append(tokens, token{tokenIdent, word, start})
				}
			} else {
				return nil, fmt.Errorf("unexpected character %q at position %d", input[i], i)
			}
		}
	}

	tokens = append(tokens, token{tokenEOF, "", i})
	return tokens, nil
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9') || b == '.'
}

// --- AST Nodes ---

type orNode struct {
	left, right Expression
}

func (n *orNode) Evaluate(e *event.Event) bool {
	return n.left.Evaluate(e) || n.right.Evaluate(e)
}

type andNode struct {
	left, right Expression
}

func (n *andNode) Evaluate(e *event.Event) bool {
	return n.left.Evaluate(e) && n.right.Evaluate(e)
}

type compareNode struct {
	field string
	op    string
	value interface{} // string, bool, or nil (null)
}

func (n *compareNode) Evaluate(e *event.Event) bool {
	fieldVal, exists := e.GetField(n.field)

	switch n.op {
	case "=":
		if n.value == nil {
			return !exists || fieldVal == nil
		}
		if !exists || fieldVal == nil {
			return false
		}
		return compareEqual(fieldVal, n.value)
	case "!=":
		if n.value == nil {
			return exists && fieldVal != nil
		}
		if !exists || fieldVal == nil {
			return true
		}
		return !compareEqual(fieldVal, n.value)
	}
	return false
}

func compareEqual(fieldVal, litVal interface{}) bool {
	switch lv := litVal.(type) {
	case string:
		fv, ok := fieldVal.(string)
		if !ok {
			return false
		}
		return fv == lv
	case bool:
		fv, ok := fieldVal.(bool)
		if !ok {
			return false
		}
		return fv == lv
	}
	return false
}

type hasNode struct {
	field string
}

func (n *hasNode) Evaluate(e *event.Event) bool {
	val, exists := e.GetField(n.field)
	return exists && val != nil
}

// --- Parser ---

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return token{typ: tokenEOF}
}

func (p *parser) advance() token {
	t := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *parser) expect(typ tokenType) (token, error) {
	t := p.advance()
	if t.typ != typ {
		return t, fmt.Errorf("expected token type %d but got %q at position %d", typ, t.value, t.pos)
	}
	return t, nil
}

// parseOr: and_expr ("OR" and_expr)*
func (p *parser) parseOr() (Expression, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().typ == tokenOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &orNode{left, right}
	}
	return left, nil
}

// parseAnd: primary ("AND" primary)*
func (p *parser) parseAnd() (Expression, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.peek().typ == tokenAnd {
		p.advance()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &andNode{left, right}
	}
	return left, nil
}

// parsePrimary: "(" expression ")" | "has" "(" field_ref ")" | field_ref op literal
func (p *parser) parsePrimary() (Expression, error) {
	t := p.peek()

	switch t.typ {
	case tokenLParen:
		p.advance()
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' at position %d", p.peek().pos)
		}
		return expr, nil

	case tokenHas:
		p.advance()
		if _, err := p.expect(tokenLParen); err != nil {
			return nil, fmt.Errorf("expected '(' after 'has' at position %d", p.peek().pos)
		}
		fieldTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, fmt.Errorf("expected field reference in has() at position %d", p.peek().pos)
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' after has() argument at position %d", p.peek().pos)
		}
		return &hasNode{field: fieldTok.value}, nil

	case tokenIdent:
		fieldTok := p.advance()
		opTok := p.advance()
		if opTok.typ != tokenEq && opTok.typ != tokenNeq {
			return nil, fmt.Errorf("expected '=' or '!=' after field reference at position %d", opTok.pos)
		}
		valTok := p.advance()
		var val interface{}
		switch valTok.typ {
		case tokenString:
			val = valTok.value
		case tokenBool:
			val = valTok.value == "true"
		case tokenNull:
			val = nil
		default:
			return nil, fmt.Errorf("expected literal value at position %d", valTok.pos)
		}
		return &compareNode{field: fieldTok.value, op: opTok.value, value: val}, nil

	default:
		return nil, fmt.Errorf("unexpected token %q at position %d", t.value, t.pos)
	}
}
