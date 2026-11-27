package translator

import (
	_ "embed" // библиотека на Lisp встраивается в транслятор
	"encoding/binary"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/panochko/csa-lab4/internal/isa"
)

//go:embed prelude.lisp
var prelude string

// Program — результат трансляции.
type Program struct {
	Image   []byte            // бинарный образ памяти: код, затем данные
	Listing string            // отладочный листинг
	Sources map[uint32]string // адрес инструкции -> исходное выражение
}

// Translate переводит исходный текст в машинный код.
func Translate(src string) (*Program, error) {
	var prog *Program
	err := recoverCompile(func() error {
		var err error
		prog, err = translate(src)
		return err
	})
	return prog, err
}

// recoverCompile превращает панику кодогенератора в обычную ошибку.
func recoverCompile(f func() error) (err error) { // именованный результат меняется из defer
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(compileError)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	return f()
}

func translate(src string) (*Program, error) {
	c := &compiler{labels: map[string]int{}, globals: map[string]bool{}, funcs: map[string]function{}, queued: map[string]bool{}}
	var top []*node
	for _, text := range []string{prelude, src} {
		forms, perr := parse(text)
		if perr != nil {
			return nil, perr
		}
		for _, f := range forms {
			if f.head() != "defun" {
				top = append(top, f)
				continue
			}
			c.define(f)
		}
	}
	c.collectGlobals(top, nil)
	for _, f := range c.funcs {
		c.collectGlobals(f.body, f.params)
	}

	c.frame(nil, top, "main")
	c.emit(isa.Halt, no, no)
	// очередь растёт по ходу компиляции: функции вызывают другие функции
	for i := 0; i < len(c.queue); i++ { //nolint:intrange // длина меняется внутри цикла
		name := c.queue[i]
		f := c.funcs[name]
		source := fmt.Sprintf("(defun %s (%s) ...)", name, strings.Join(f.params, " "))
		c.mark("f:" + name)
		c.source = source
		c.emit(isa.Push, fp, no)
		c.frame(f.params, f.body, source)
		c.emit(isa.Mov, fp, sp)
		c.emit(isa.Pop, no, fp)
		c.emit(isa.Ret, no, no)
	}
	return c.link()
}

func (c *compiler) define(f *node) {
	if len(f.items) < 3 || f.items[1].kind != kSymbol || f.items[2].kind != kList {
		failf("defun: %s", f)
	}
	params := make([]string, 0, len(f.items[2].items))
	for _, p := range f.items[2].items {
		if p.kind != kSymbol {
			failf("defun: параметр должен быть именем: %s", p)
		}
		params = append(params, p.text)
	}
	c.funcs[f.items[1].text] = function{params: params, body: f.items[3:]}
}

// collectGlobals: всё, чему присваивают вне локальной области, — глобальная переменная.
func (c *compiler) collectGlobals(forms []*node, locals []string) {
	for _, n := range forms {
		if n.kind != kList {
			continue
		}
		inner := locals
		switch n.head() {
		case "setq":
			if len(n.items) == 3 && !slices.Contains(locals, n.items[1].text) {
				c.globals[n.items[1].text] = true
			}
		case "let":
			if len(n.items) > 1 {
				for _, b := range n.items[1].items {
					if len(b.items) > 0 {
						inner = append(inner[:len(inner):len(inner)], b.items[0].text)
					}
				}
			}
		}
		c.collectGlobals(n.items, inner)
	}
}

// link назначает адреса, подставляет их в операнды и формирует образ и листинг.
func (c *compiler) link() (*Program, error) {
	addr := map[string]uint32{}
	lineAddr := make([]uint32, len(c.code)+1)
	var pc uint32
	for i, l := range c.code {
		lineAddr[i] = pc
		pc += uint32(len(l.instruction(addr).Encode()) * isa.WordSize)
	}
	lineAddr[len(c.code)] = pc
	for label, i := range c.labels {
		addr[label] = lineAddr[i]
	}
	globals := slices.Sorted(maps.Keys(c.globals))
	for _, name := range globals {
		addr["g:"+name] = pc
		pc += isa.WordSize
	}
	for _, d := range c.data {
		addr[d.label] = pc
		pc += uint32(len(d.words) * isa.WordSize)
	}
	if pc > isa.MemorySize/2 {
		return nil, fmt.Errorf("программа занимает %d байт и не оставляет места для стека", pc)
	}

	p := &Program{Sources: map[uint32]string{}}
	var listing strings.Builder
	listing.WriteString("# code\n")
	put := func(words []uint32) string {
		hex := make([]string, len(words))
		for i, w := range words {
			p.Image = binary.LittleEndian.AppendUint32(p.Image, w)
			hex[i] = fmt.Sprintf("%08X", w)
		}
		return strings.Join(hex, " ")
	}
	for i, l := range c.code {
		ins := l.instruction(addr)
		p.Sources[lineAddr[i]] = l.source
		fmt.Fprintf(&listing, "%04X - %s - %s ; %s\n", lineAddr[i], put(ins.Encode()), ins, l.source)
	}
	listing.WriteString("# data\n")
	for _, name := range globals {
		fmt.Fprintf(&listing, "%04X - %s - var %s\n", addr["g:"+name], put([]uint32{0}), name)
	}
	for _, d := range c.data {
		hex := put(d.words)
		if len(d.words) > 4 {
			hex = fmt.Sprintf("%s ... (%d words)", hex[:35], len(d.words))
		}
		fmt.Fprintf(&listing, "%04X - %s - %s\n", addr[d.label], hex, shorten(d.note))
	}
	p.Listing = listing.String()
	return p, nil
}
