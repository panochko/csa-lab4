// Package machine — потактовая модель CISC-процессора с микрокодом.
package machine

import (
	"errors"
	"fmt"
	"math/bits"

	"github.com/panochko/csa-lab4/internal/isa"
)

// ErrHalted — выполнена инструкция halt.
var ErrHalted = errors.New("halt")

// ErrInputEmpty — чтение из пустого буфера ввода.
var ErrInputEmpty = errors.New("input buffer is empty")

type flags struct{ n, z, v, c bool }

// Machine — состояние DataPath и ControlUnit.
type Machine struct {
	rom   *microROM
	cache cache

	regs               [isa.RegCount]uint32
	pc, ir, ar, dr, br uint32
	spec, cnt          uint8
	nzvc               flags
	mpc, mra           int

	input  []byte
	output []byte
	ticks  int
	instrs int

	instrAddr uint32 // адрес текущей инструкции, только для журнала
	event     string // событие памяти или ввода-вывода на последнем такте
}

// New создаёт машину с образом памяти и буфером ввода.
func New(image, input []byte, useCache bool) (*Machine, error) {
	if len(image) > isa.MemorySize {
		return nil, fmt.Errorf("образ %d байт не помещается в память", len(image))
	}
	mem := make([]byte, isa.MemorySize)
	copy(mem, image)
	m := &Machine{rom: buildROM(), input: input}
	m.cache = cache{enabled: useCache, mem: mem}
	m.regs[isa.SP] = isa.MemorySize
	return m, nil
}

// Output возвращает всё выведенное.
func (m *Machine) Output() []byte { return m.output }

// Ticks возвращает число прошедших тактов.
func (m *Machine) Ticks() int { return m.ticks }

// Instructions возвращает число начатых инструкций.
func (m *Machine) Instructions() int { return m.instrs }

// Stats возвращает статистику кэша.
func (m *Machine) Stats() CacheStats { return m.cache.stats }

func (m *Machine) regIndex(s regSel) uint8 {
	switch s {
	case regSP:
		return isa.SP
	case regNary:
		return uint8(m.ir>>16) & 7
	case regSpec:
	}
	return m.spec & 7
}

func (m *Machine) opcode() isa.Opcode { return isa.Opcode(m.ir >> 24) }

func (m *Machine) left(u micro) uint32 {
	switch u.a {
	case aReg:
		return m.regs[m.regIndex(u.reg)]
	case aPC:
		return m.pc
	case aDR:
		return m.dr
	case aZero:
	}
	return 0
}

func (m *Machine) right(u micro) uint32 {
	switch u.b {
	case bBR:
		return m.br
	case bDR:
		return m.dr
	case bFour:
		return isa.WordSize
	case bNeutral:
		if m.opcode() == isa.Muln {
			return 1
		}
	case bZero:
	}
	return 0
}

func addFlags(a, b, carry uint32) (uint32, flags) {
	res, c := bits.Add32(a, b, carry)
	v := (^(a ^ b) & (a ^ res) >> 31) != 0
	return res, flags{n: int32(res) < 0, z: res == 0, v: v, c: c != 0}
}

func subFlags(a, b, borrow uint32) (uint32, flags) {
	res, c := bits.Sub32(a, b, borrow)
	v := (((a ^ b) & (a ^ res)) >> 31) != 0
	return res, flags{n: int32(res) < 0, z: res == 0, v: v, c: c != 0}
}

func logicFlags(res uint32) (uint32, flags) {
	return res, flags{n: int32(res) < 0, z: res == 0}
}

func bit(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// alu — арифметико-логическое устройство.
func (m *Machine) alu(u micro, a, b uint32) (uint32, flags, error) {
	switch u.alu {
	case aluAdd:
		r, f := addFlags(a, b, 0)
		return r, f, nil
	case aluSub:
		r, f := subFlags(a, b, 0)
		return r, f, nil
	case aluOpcode:
	}
	var r uint32
	var f flags
	switch op := m.opcode(); op {
	case isa.Add, isa.Addn:
		r, f = addFlags(a, b, 0)
	case isa.Adc:
		r, f = addFlags(a, b, bit(m.nzvc.c))
	case isa.Sub:
		r, f = subFlags(a, b, 0)
	case isa.Sbc:
		r, f = subFlags(a, b, bit(m.nzvc.c))
	case isa.Mul, isa.Muln:
		r, f = logicFlags(a * b)
	case isa.Mulh:
		hi, _ := bits.Mul32(a, b)
		r, f = logicFlags(hi)
	case isa.Div, isa.Mod:
		if b == 0 {
			return 0, f, errors.New("деление на ноль")
		}
		if op == isa.Div {
			r, f = logicFlags(uint32(int32(a) / int32(b)))
		} else {
			r, f = logicFlags(uint32(int32(a) % int32(b)))
		}
	case isa.And:
		r, f = logicFlags(a & b)
	case isa.Or:
		r, f = logicFlags(a | b)
	case isa.Xor:
		r, f = logicFlags(a ^ b)
	case isa.Shr:
		r, f = logicFlags(a >> (b & 31))
	default:
		return 0, f, fmt.Errorf("АЛУ: неожиданный опкод %s", op)
	}
	return r, f, nil
}

func (m *Machine) condition() bool {
	f := m.nzvc
	switch m.opcode() {
	case isa.Jmp:
		return true
	case isa.Jz:
		return f.z
	case isa.Jnz:
		return !f.z
	case isa.Jl:
		return f.n != f.v
	case isa.Jle:
		return f.z || f.n != f.v
	case isa.Jg:
		return !f.z && f.n == f.v
	case isa.Jge:
		return f.n == f.v
	case isa.Jc:
		return f.c
	default:
		return false
	}
}

// Tick выполняет ровно один такт. Пока память не готова, защёлки не
// срабатывают и mPC не меняется — процессор стоит на той же микрокоманде.
func (m *Machine) Tick() error {
	u := m.rom.code[m.mpc]
	m.event = ""
	if m.mpc == fetchAddr {
		m.instrAddr = m.pc
		m.instrs++
	}
	m.ticks++

	var memData uint32
	if u.mem != memNone {
		if err := m.cache.check(m.ar); err != nil {
			return err
		}
	}
	switch u.mem {
	case memRead:
		value, ready, event := m.cache.read(m.ar)
		m.event = event
		if !ready {
			return nil
		}
		memData = value
	case memWrite:
		ready, event := m.cache.write(m.ar, m.dr)
		m.event = event
		if !ready {
			return nil
		}
	case memNone:
	}

	var portData uint32
	if u.dr == drPort {
		if len(m.input) == 0 {
			return ErrInputEmpty
		}
		portData, m.input = uint32(m.input[0]), m.input[1:]
		m.event = fmt.Sprintf("IN port %d -> %q", uint8(m.ir), byte(portData))
	}
	if u.out {
		m.output = append(m.output, byte(m.br))
		m.event = fmt.Sprintf("OUT port %d <- %q", uint8(m.ir), byte(m.br))
	}

	if err := m.latch(u, memData, portData); err != nil {
		return err
	}
	return m.sequence(u)
}

// latch вычисляет комбинационные схемы по старым значениям регистров
// и защёлкивает результаты.
func (m *Machine) latch(u micro, memData, portData uint32) error {
	var res uint32
	var f flags
	if u.latch&(lReg|lPC|lPCCond|lIR|lAR|lDR|lBR|lFlags) != 0 {
		var err error
		if res, f, err = m.alu(u, m.left(u), m.right(u)); err != nil {
			return err
		}
	}
	// шина данных: память, порт или результат АЛУ
	bus := res
	switch u.dr {
	case drMem:
		bus = memData
	case drPort:
		bus = portData
	case drALU:
	}
	spec := [...]uint8{uint8(m.ir >> 16), uint8(m.ir >> 8), uint8(m.dr >> 24)}[u.spec]
	reg := m.regIndex(u.reg)
	taken := u.latch&lPCCond != 0 && m.condition()

	has := func(l latch) bool { return u.latch&l != 0 }
	if has(lReg) {
		m.regs[reg] = res
	}
	if has(lPC) || taken {
		m.pc = res
	}
	if has(lPCInc) {
		m.pc += isa.WordSize
	}
	if has(lAR) {
		m.ar = res
	}
	if has(lBR) {
		m.br = bus
	}
	if has(lFlags) {
		m.nzvc = f
	}
	if has(lSpec) {
		m.spec = spec
	}
	if has(lCntLoad) {
		m.cnt = uint8(m.ir >> 8)
	}
	if has(lCntDec) {
		m.cnt--
	}
	if has(lIR) {
		m.ir = res
	}
	if has(lDR) {
		m.dr = bus
	}
	return nil
}

// sequence — выбор адреса следующей микрокоманды.
func (m *Machine) sequence(u micro) error {
	mode := isa.Mode(m.spec >> 4)
	switch u.seq {
	case seqNext:
		m.mpc++
	case seqJump:
		m.mpc = u.addr
	case seqOpcode:
		op := m.opcode()
		if op >= isa.OpcodeCount {
			return fmt.Errorf("неизвестный опкод %#x", uint8(op))
		}
		m.mpc = m.rom.opTable[op]
	case seqSrc:
		if mode == isa.None || mode >= isa.ModeCount {
			return fmt.Errorf("неверный режим источника %d", mode)
		}
		m.mra, m.mpc = m.mpc+1, m.rom.srcTable[mode]
	case seqDst:
		switch mode {
		case isa.Reg:
			m.mpc++
		case isa.Abs, isa.Ind, isa.Based:
			m.mra, m.mpc = m.mpc+2, m.rom.dstTable[mode]
		case isa.None, isa.Imm, isa.ModeCount:
			return fmt.Errorf("неверный режим приёмника %d", mode)
		}
	case seqReturn:
		m.mpc = m.mra
	case seqCntLoop:
		if m.cnt != 0 {
			m.mpc = u.addr
		} else {
			m.mpc = fetchAddr
		}
	case seqHalt:
		return ErrHalted
	}
	return nil
}
