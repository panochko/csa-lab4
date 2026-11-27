// Package isa описывает систему команд: опкоды, режимы адресации и кодирование.
package isa

import (
	"fmt"
	"strconv"
	"strings"
)

// Opcode — код операции (старший байт заголовка инструкции).
type Opcode uint8

// Опкоды.
const (
	Halt Opcode = iota
	Mov
	Add
	Adc
	Sub
	Sbc
	Mul
	Mulh
	Div
	Mod
	And
	Or
	Xor
	Cmp
	Addn
	Muln
	Jmp
	Jz
	Jnz
	Jl
	Jle
	Jg
	Jge
	Jc
	Call
	Ret
	Push
	Pop
	In
	Out
	Shr
	OpcodeCount
)

var names = [OpcodeCount]string{
	"halt", "mov", "add", "adc", "sub", "sbc", "mul", "mulh", "div", "mod",
	"and", "or", "xor", "cmp", "addn", "muln", "jmp", "jz", "jnz", "jl",
	"jle", "jg", "jge", "jc", "call", "ret", "push", "pop", "in", "out", "shr",
}

func (o Opcode) String() string {
	if o < OpcodeCount {
		return names[o]
	}
	return fmt.Sprintf("op%02X", uint8(o))
}

// Mode — режим адресации.
type Mode uint8

// Режимы адресации.
const (
	None Mode = iota
	Reg
	Imm
	Abs
	Ind
	Based
	ModeCount
)

// HasExt сообщает, занимает ли операнд слово расширения.
func (m Mode) HasExt() bool { return m == Imm || m == Abs || m == Based }

// Номера регистров.
const (
	R0 uint8 = iota
	R1
	R2
	R3
	R4
	R5
	SP
	FP
	RegCount
)

var regNames = [RegCount]string{"r0", "r1", "r2", "r3", "r4", "r5", "sp", "fp"}

// Порты ввода-вывода.
const (
	PortIn  uint8 = 0
	PortOut uint8 = 1
)

// WordSize — размер машинного слова в байтах.
const WordSize = 4

// MemorySize — объём памяти в байтах.
const MemorySize = 1 << 16

// Operand — операнд инструкции.
type Operand struct {
	Mode Mode
	Reg  uint8
	Ext  uint32
}

// Spec возвращает байт-описатель операнда.
func (o Operand) Spec() uint8 { return uint8(o.Mode)<<4 | o.Reg }

func (o Operand) String() string {
	switch o.Mode {
	case Reg:
		return regNames[o.Reg&7]
	case Imm:
		return fmt.Sprintf("#%d", int32(o.Ext))
	case Abs:
		return fmt.Sprintf("[%d]", o.Ext)
	case Ind:
		return fmt.Sprintf("[%s]", regNames[o.Reg&7])
	case Based:
		return fmt.Sprintf("[%s%+d]", regNames[o.Reg&7], int32(o.Ext))
	case None, ModeCount:
	}
	return "?"
}

// Instruction — разобранная инструкция.
type Instruction struct {
	Op   Opcode
	Src  Operand
	Dst  Operand
	Port uint8
	Args []Operand // операнды addn/muln
}

// IsNary сообщает, имеет ли инструкция переменное число операндов.
func (i Instruction) IsNary() bool { return i.Op == Addn || i.Op == Muln }

// Encode переводит инструкцию в машинные слова.
func (i Instruction) Encode() []uint32 {
	if i.IsNary() {
		words := []uint32{header(i.Op, i.Dst.Spec(), uint8(len(i.Args)), 0)}
		for _, a := range i.Args {
			words = append(words, uint32(a.Spec())<<24)
			if a.Mode.HasExt() {
				words = append(words, a.Ext)
			}
		}
		return words
	}
	words := []uint32{header(i.Op, i.Src.Spec(), i.Dst.Spec(), i.Port)}
	for _, a := range []Operand{i.Src, i.Dst} {
		if a.Mode.HasExt() {
			words = append(words, a.Ext)
		}
	}
	return words
}

func header(op Opcode, s1, s2, s3 uint8) uint32 {
	return uint32(op)<<24 | uint32(s1)<<16 | uint32(s2)<<8 | uint32(s3)
}

func operand(spec uint8, next func() uint32) Operand {
	o := Operand{Mode: Mode(spec >> 4), Reg: spec & 0xF}
	if o.Mode.HasExt() {
		o.Ext = next()
	}
	return o
}

// Decode разбирает инструкцию; next выдаёт очередное слово.
// Используется только для листинга и журнала, не процессором.
func Decode(next func() uint32) Instruction {
	h := next()
	i := Instruction{Op: Opcode(h >> 24)}
	s1, s2 := uint8(h>>16), uint8(h>>8)
	if i.IsNary() {
		i.Dst = Operand{Mode: Mode(s1 >> 4), Reg: s1 & 0xF}
		for range int(s2) {
			spec := uint8(next() >> 24)
			i.Args = append(i.Args, operand(spec, next))
		}
		return i
	}
	i.Port = uint8(h)
	i.Src = operand(s1, next)
	i.Dst = operand(s2, next)
	return i
}

func (i Instruction) String() string {
	if i.IsNary() {
		parts := []string{i.Dst.String(), strconv.Itoa(len(i.Args))}
		for _, a := range i.Args {
			parts = append(parts, a.String())
		}
		return i.Op.String() + " " + strings.Join(parts, ", ")
	}
	var parts []string
	if i.Op == In {
		parts = append(parts, strconv.Itoa(int(i.Port)))
	}
	if i.Src.Mode != None {
		parts = append(parts, i.Src.String())
	}
	if i.Dst.Mode != None {
		parts = append(parts, i.Dst.String())
	}
	if i.Op == Out {
		parts = append(parts, strconv.Itoa(int(i.Port)))
	}
	return strings.TrimSpace(i.Op.String() + " " + strings.Join(parts, ", "))
}
