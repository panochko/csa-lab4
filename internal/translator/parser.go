// Package translator переводит программу на Lisp в машинный код.
package translator

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type kind int

const (
	kList kind = iota
	kNumber
	kSymbol
	kString
)

// node — узел синтаксического дерева.
type node struct {
	kind  kind
	num   int32
	text  string // имя символа или содержимое строки
	items []*node
}

// head возвращает имя оператора для списка вида (name ...).
func (n *node) head() string {
	if n.kind == kList && len(n.items) > 0 && n.items[0].kind == kSymbol {
		return n.items[0].text
	}
	return ""
}

func (n *node) String() string {
	switch n.kind {
	case kNumber:
		return strconv.Itoa(int(n.num))
	case kSymbol:
		return n.text
	case kString:
		return strconv.Quote(n.text)
	case kList:
	}
	parts := make([]string, len(n.items))
	for i, c := range n.items {
		parts[i] = c.String()
	}
	return "(" + strings.Join(parts, " ") + ")"
}

type parser struct {
	src []rune
	pos int
}

func parse(src string) ([]*node, error) {
	p := &parser{src: []rune(src)}
	var forms []*node
	for {
		p.skip()
		if p.pos >= len(p.src) {
			return forms, nil
		}
		n, err := p.expr()
		if err != nil {
			return nil, err
		}
		forms = append(forms, n)
	}
}

func (p *parser) skip() {
	for p.pos < len(p.src) {
		switch {
		case p.src[p.pos] == ';':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		case unicode.IsSpace(p.src[p.pos]):
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) expr() (*node, error) {
	p.skip()
	if p.pos >= len(p.src) {
		return nil, errors.New("неожиданный конец файла")
	}
	switch p.src[p.pos] {
	case '(':
		p.pos++
		n := &node{kind: kList}
		for {
			p.skip()
			if p.pos < len(p.src) && p.src[p.pos] == ')' {
				p.pos++
				return n, nil
			}
			child, err := p.expr()
			if err != nil {
				return nil, err
			}
			n.items = append(n.items, child)
		}
	case ')':
		return nil, errors.New("лишняя закрывающая скобка")
	case '"':
		return p.str()
	}
	return p.atom()
}

func (p *parser) str() (*node, error) {
	var b strings.Builder
	for p.pos++; p.pos < len(p.src); p.pos++ {
		c := p.src[p.pos]
		if c == '"' {
			p.pos++
			return &node{kind: kString, text: b.String()}, nil
		}
		if c == '\\' && p.pos+1 < len(p.src) {
			p.pos++
			c = p.src[p.pos]
			if c == 'n' {
				c = '\n'
			}
		}
		if c > 0xFF {
			return nil, fmt.Errorf("символ %q не помещается в байт", c)
		}
		b.WriteByte(byte(c))
	}
	return nil, errors.New("не закрыта строка")
}

var charNames = map[string]int32{"newline": '\n', "space": ' ', "tab": '\t'}

func (p *parser) atom() (*node, error) {
	start := p.pos
	// символьный литерал: #\x, где x может быть скобкой
	if p.src[p.pos] == '#' && p.pos+2 < len(p.src) && p.src[p.pos+1] == '\\' {
		p.pos += 3
	}
	for p.pos < len(p.src) && !unicode.IsSpace(p.src[p.pos]) && !strings.ContainsRune("();", p.src[p.pos]) {
		p.pos++
	}
	text := string(p.src[start:p.pos])
	if name, ok := strings.CutPrefix(text, `#\`); ok {
		if v, found := charNames[name]; found {
			return &node{kind: kNumber, num: v}, nil
		}
		if len(name) != 1 {
			return nil, fmt.Errorf("неизвестный символ %s", text)
		}
		return &node{kind: kNumber, num: int32(name[0])}, nil
	}
	if v, err := strconv.ParseInt(text, 0, 64); err == nil {
		if v < -1<<31 || v >= 1<<32 {
			return nil, fmt.Errorf("число %s не помещается в слово", text)
		}
		return &node{kind: kNumber, num: int32(v)}, nil
	}
	return &node{kind: kSymbol, text: text}, nil
}
