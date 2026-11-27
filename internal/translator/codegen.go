package translator

import (
	"fmt"
	"slices"
	"strings"

	"github.com/panochko/csa-lab4/internal/isa"
)

// operand — операнд, значение которого может зависеть от ещё неизвестного адреса.
type operand struct {
	isa.Operand

	label string
}

type line struct {
	op       isa.Opcode
	src, dst operand
	args     []operand
	port     uint8
	source   string
}

func (l line) instruction(addr map[string]uint32) isa.Instruction {
	resolve := func(o operand) isa.Operand {
		if o.label != "" {
			o.Ext = addr[o.label]
		}
		return o.Operand
	}
	i := isa.Instruction{Op: l.op, Src: resolve(l.src), Dst: resolve(l.dst), Port: l.port}
	for _, a := range l.args {
		i.Args = append(i.Args, resolve(a))
	}
	return i
}

func reg(r uint8) operand       { return operand{Operand: isa.Operand{Mode: isa.Reg, Reg: r}} }
func imm(v int32) operand       { return operand{Operand: isa.Operand{Mode: isa.Imm, Ext: uint32(v)}} }
func immLabel(l string) operand { return operand{Operand: isa.Operand{Mode: isa.Imm}, label: l} }
func absLabel(l string) operand { return operand{Operand: isa.Operand{Mode: isa.Abs}, label: l} }
func ind(r uint8) operand       { return operand{Operand: isa.Operand{Mode: isa.Ind, Reg: r}} }

func based(r uint8, off int) operand {
	return operand{Operand: isa.Operand{Mode: isa.Based, Reg: r, Ext: uint32(int32(off))}}
}

var (
	r0 = reg(isa.R0)
	r1 = reg(isa.R1)
	sp = reg(isa.SP)
	fp = reg(isa.FP)
	no = operand{}
)

type function struct {
	params []string
	body   []*node
}

type dataItem struct {
	label string
	words []uint32
	note  string
}

type compiler struct {
	code    []line
	labels  map[string]int // метка -> номер строки кода
	data    []dataItem
	globals map[string]bool
	funcs   map[string]function
	queue   []string // функции, которые нужно скомпилировать
	queued  map[string]bool
	scope   []map[string]int // имя -> смещение от FP
	slots   int
	maxSlot int
	nlabel  int
	source  string
}

func failf(format string, args ...any) { panic(compileError(fmt.Sprintf(format, args...))) }

type compileError string

func (e compileError) Error() string { return string(e) }

func (c *compiler) emit(op isa.Opcode, src, dst operand) {
	c.code = append(c.code, line{op: op, src: src, dst: dst, source: c.source})
}

func (c *compiler) newLabel() string {
	c.nlabel++
	return fmt.Sprintf("L%d", c.nlabel)
}

func (c *compiler) mark(label string) { c.labels[label] = len(c.code) }

func (c *compiler) jump(op isa.Opcode, label string) { c.emit(op, immLabel(label), no) }

func (c *compiler) addData(words []uint32, note string) string {
	label := fmt.Sprintf("D%d", len(c.data))
	c.data = append(c.data, dataItem{label: label, words: words, note: note})
	return label
}

func (c *compiler) lookup(name string) (operand, bool) {
	for i := len(c.scope) - 1; i >= 0; i-- {
		if off, ok := c.scope[i][name]; ok {
			return based(isa.FP, off), true
		}
	}
	if c.globals[name] {
		return absLabel("g:" + name), true
	}
	return no, false
}

// simple возвращает операнд для выражения, которое не требует вычислений:
// такие значения CISC-команды берут прямо из памяти или из самой команды.
func (c *compiler) simple(n *node) (operand, bool) {
	switch n.kind {
	case kNumber:
		return imm(n.num), true
	case kSymbol:
		if o, ok := c.lookup(n.text); ok {
			return o, true
		}
		failf("неизвестная переменная %s", n.text)
	case kString:
		words := []uint32{uint32(len(n.text))}
		for _, b := range []byte(n.text) {
			words = append(words, uint32(b))
		}
		return immLabel(c.addData(words, "pstr "+n.String())), true
	case kList:
		if n.head() == "array" {
			if len(n.items) != 2 || n.items[1].kind != kNumber || n.items[1].num <= 0 {
				failf("array требует положительный числовой литерал: %s", n)
			}
			return immLabel(c.addData(make([]uint32, n.items[1].num), n.String())), true
		}
	}
	return no, false
}

var arithmetic = map[string]isa.Opcode{
	"+": isa.Add, "-": isa.Sub, "*": isa.Mul, "/": isa.Div, "mod": isa.Mod,
	"mulh": isa.Mulh, "logand": isa.And, "logior": isa.Or, "logxor": isa.Xor, "shr": isa.Shr,
}

// переход, выполняемый когда сравнение ИСТИННО, и обратный ему.
var comparisons = map[string][2]isa.Opcode{
	"=": {isa.Jz, isa.Jnz}, "/=": {isa.Jnz, isa.Jz},
	"<": {isa.Jl, isa.Jge}, ">=": {isa.Jge, isa.Jl},
	">": {isa.Jg, isa.Jle}, "<=": {isa.Jle, isa.Jg},
}

func arity(n *node, want int) []*node {
	args := n.items[1:]
	if len(args) != want {
		failf("%s: ожидалось аргументов %d", n, want)
	}
	return args
}

// expr компилирует выражение; результат всегда оказывается в R0.
func (c *compiler) expr(n *node) {
	if o, ok := c.simple(n); ok {
		c.emit(isa.Mov, o, r0)
		return
	}
	if len(n.items) == 0 || n.head() == "" {
		failf("ожидался вызов: %s", n)
	}
	saved := c.source
	c.source = shorten(n.String())
	defer func() { c.source = saved }()

	name, args := n.head(), n.items[1:]
	if op, ok := arithmetic[name]; ok {
		c.arithmetic(n, op)
		return
	}
	if _, ok := comparisons[name]; ok {
		c.boolean(n)
		return
	}
	switch name {
	case "setq":
		c.setq(n)
	case "if":
		c.ifForm(n)
	case "while":
		c.while(n)
	case "progn":
		c.body(args)
	case "let":
		c.let(n)
	case "and":
		c.shortCircuit(args, isa.Jz, 1)
	case "or":
		c.shortCircuit(args, isa.Jnz, 0)
	case "not":
		arity(n, 1)
		c.boolean(n)
	case "load":
		c.expr(arity(n, 1)[0])
		c.emit(isa.Mov, ind(isa.R0), r0)
	case "store":
		c.twoValues(arity(n, 2)) // R0 = адрес, R1 = значение
		c.emit(isa.Mov, r1, ind(isa.R0))
		c.emit(isa.Mov, r1, r0)
	case "add64":
		c.wide(n, isa.Add, isa.Adc)
	case "sub64":
		c.wide(n, isa.Sub, isa.Sbc)
	case "read-char":
		arity(n, 0)
		c.code = append(c.code, line{op: isa.In, dst: r0, port: isa.PortIn, source: c.source})
	case "print-char":
		c.expr(arity(n, 1)[0])
		c.code = append(c.code, line{op: isa.Out, src: r0, port: isa.PortOut, source: c.source})
	case "defun":
		failf("defun допускается только на верхнем уровне")
	default:
		c.call(name, args)
	}
}

// twoValues вычисляет a и b слева направо: a -> R0, b -> R1.
func (c *compiler) twoValues(args []*node) {
	c.expr(args[0])
	if o, ok := c.simple(args[1]); ok {
		c.emit(isa.Mov, o, r1)
		return
	}
	c.emit(isa.Push, r0, no)
	c.expr(args[1])
	c.emit(isa.Mov, r0, r1)
	c.emit(isa.Pop, no, r0)
}

// binary применяет op к a и b, результат (или флаги) относится к R0.
func (c *compiler) binary(op isa.Opcode, a, b *node) {
	c.expr(a)
	if o, ok := c.simple(b); ok {
		c.emit(op, o, r0)
		return
	}
	c.emit(isa.Push, r0, no)
	c.expr(b)
	c.emit(isa.Mov, r0, r1)
	c.emit(isa.Pop, no, r0)
	c.emit(op, r1, r0)
}

func (c *compiler) arithmetic(n *node, op isa.Opcode) {
	args := n.items[1:]
	variadic := op == isa.Add || op == isa.Mul
	switch {
	case len(args) == 1 && op == isa.Sub:
		c.binary(isa.Sub, &node{kind: kNumber}, args[0])
	case variadic && len(args) > 2:
		c.nary(op, args)
	default:
		arity(n, 2)
		c.binary(op, args[0], args[1])
	}
}

// nary транслирует (+ a b c ...) в одну инструкцию addn/muln.
func (c *compiler) nary(op isa.Opcode, args []*node) {
	if len(args) > 255 {
		failf("слишком много аргументов: %d", len(args))
	}
	l := line{op: isa.Addn, dst: r0, source: c.source}
	if op == isa.Mul {
		l.op = isa.Muln
	}
	allSimple := !slices.ContainsFunc(args, func(a *node) bool { return a.kind == kList && a.head() != "array" })
	if allSimple {
		for _, a := range args {
			o, _ := c.simple(a)
			l.args = append(l.args, o)
		}
		c.code = append(c.code, l)
		return
	}
	// сложные аргументы вычисляются слева направо и складываются на стек
	for _, a := range args {
		c.expr(a)
		c.emit(isa.Push, r0, no)
	}
	for i := range args {
		l.args = append(l.args, based(isa.SP, (len(args)-1-i)*isa.WordSize))
	}
	c.code = append(c.code, l)
	c.emit(isa.Add, imm(int32(len(args)*isa.WordSize)), sp)
}

// branch переходит на label, если условие ложно.
func (c *compiler) branch(cond *node, label string) {
	if jumps, ok := comparisons[cond.head()]; ok {
		args := arity(cond, 2)
		c.binary(isa.Cmp, args[0], args[1])
		c.jump(jumps[1], label)
		return
	}
	if cond.head() == "not" {
		c.expr(arity(cond, 1)[0])
		c.emit(isa.Cmp, imm(0), r0)
		c.jump(isa.Jnz, label)
		return
	}
	c.expr(cond)
	c.emit(isa.Cmp, imm(0), r0)
	c.jump(isa.Jz, label)
}

// boolean превращает условие в значение 1 или 0.
func (c *compiler) boolean(n *node) {
	otherwise, end := c.newLabel(), c.newLabel()
	c.branch(n, otherwise)
	c.emit(isa.Mov, imm(1), r0)
	c.jump(isa.Jmp, end)
	c.mark(otherwise)
	c.emit(isa.Mov, imm(0), r0)
	c.mark(end)
}

func (c *compiler) shortCircuit(args []*node, exit isa.Opcode, empty int32) {
	end := c.newLabel()
	c.emit(isa.Mov, imm(empty), r0)
	for _, a := range args {
		c.expr(a)
		c.emit(isa.Cmp, imm(0), r0)
		c.jump(exit, end)
	}
	c.mark(end)
}

func (c *compiler) setq(n *node) {
	args := arity(n, 2)
	if args[0].kind != kSymbol {
		failf("setq: ожидалось имя: %s", n)
	}
	target, ok := c.lookup(args[0].text)
	if !ok {
		failf("неизвестная переменная %s", args[0].text)
	}
	// (setq x (+ x y)) -> одна команда с приёмником в памяти
	v := args[1]
	if op, isArith := arithmetic[v.head()]; isArith && len(v.items) == 3 && v.items[1].kind == kSymbol && v.items[1].text == args[0].text {
		if o, isSimple := c.simple(v.items[2]); isSimple {
			c.emit(op, o, target)
			c.emit(isa.Mov, target, r0)
			return
		}
	}
	c.expr(v)
	c.emit(isa.Mov, r0, target)
}

func (c *compiler) ifForm(n *node) {
	if len(n.items) != 3 && len(n.items) != 4 {
		failf("if: %s", n)
	}
	otherwise, end := c.newLabel(), c.newLabel()
	c.branch(n.items[1], otherwise)
	c.expr(n.items[2])
	c.jump(isa.Jmp, end)
	c.mark(otherwise)
	if len(n.items) == 4 {
		c.expr(n.items[3])
	} else {
		c.emit(isa.Mov, imm(0), r0)
	}
	c.mark(end)
}

// while возвращает значение тела на последней итерации или 0.
func (c *compiler) while(n *node) {
	if len(n.items) < 2 {
		failf("while: %s", n)
	}
	loop, end := c.newLabel(), c.newLabel()
	slot := c.allocSlot()
	result := based(isa.FP, slot)
	c.emit(isa.Mov, imm(0), result)
	c.mark(loop)
	c.branch(n.items[1], end)
	c.body(n.items[2:])
	c.emit(isa.Mov, r0, result)
	c.jump(isa.Jmp, loop)
	c.mark(end)
	c.emit(isa.Mov, result, r0)
	c.slots--
}

func (c *compiler) allocSlot() int {
	c.slots++
	c.maxSlot = max(c.maxSlot, c.slots)
	return -c.slots * isa.WordSize
}

func (c *compiler) body(forms []*node) {
	if len(forms) == 0 {
		c.emit(isa.Mov, imm(0), r0)
	}
	for _, f := range forms {
		c.expr(f)
	}
}

// let вычисляет значения последовательно, как let* в Common Lisp.
func (c *compiler) let(n *node) {
	if len(n.items) < 2 || n.items[1].kind != kList {
		failf("let: %s", n)
	}
	before := c.slots
	depth := len(c.scope)
	for _, b := range n.items[1].items {
		if b.kind != kList || len(b.items) != 2 || b.items[0].kind != kSymbol {
			failf("let: неверная привязка %s", b)
		}
		c.expr(b.items[1])
		off := c.allocSlot()
		c.emit(isa.Mov, r0, based(isa.FP, off))
		c.scope = append(c.scope, map[string]int{b.items[0].text: off})
	}
	c.body(n.items[2:])
	c.scope, c.slots = c.scope[:depth], before
}

// wide — операция над 64-битными числами в памяти: [lo, hi].
func (c *compiler) wide(n *node, low, high isa.Opcode) {
	c.twoValues(arity(n, 2)) // R0 = приёмник, R1 = источник
	c.emit(low, ind(isa.R1), ind(isa.R0))
	c.emit(high, based(isa.R1, isa.WordSize), based(isa.R0, isa.WordSize))
}

func (c *compiler) call(name string, args []*node) {
	f, ok := c.funcs[name]
	if !ok {
		failf("неизвестная функция %s", name)
	}
	if len(f.params) != len(args) {
		failf("%s: ожидалось аргументов %d, передано %d", name, len(f.params), len(args))
	}
	for _, a := range args {
		if o, isSimple := c.simple(a); isSimple {
			c.emit(isa.Push, o, no)
			continue
		}
		c.expr(a)
		c.emit(isa.Push, r0, no)
	}
	c.require(name)
	c.emit(isa.Call, immLabel("f:"+name), no)
	if len(args) > 0 {
		c.emit(isa.Add, imm(int32(len(args)*isa.WordSize)), sp)
	}
}

// require ставит функцию в очередь: неиспользуемые в бинарь не попадают.
func (c *compiler) require(name string) {
	if !c.queued[name] {
		c.queued[name] = true
		c.queue = append(c.queue, name)
	}
}

// frame компилирует тело с прологом, резервирующим место под локальные.
func (c *compiler) frame(params []string, body []*node, source string) {
	c.source = source
	scope := map[string]int{}
	for i, p := range params {
		scope[p] = 2*isa.WordSize + (len(params)-1-i)*isa.WordSize
	}
	c.scope, c.slots, c.maxSlot = []map[string]int{scope}, 0, 0
	c.emit(isa.Mov, sp, fp)
	reserve := len(c.code)
	c.emit(isa.Sub, imm(0), sp)
	c.body(body)
	c.source = source
	c.code[reserve].src = imm(int32(c.maxSlot * isa.WordSize))
}

func shorten(s string) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	if r := []rune(s); len(r) > 60 {
		return string(r[:57]) + "..."
	}
	return s
}
