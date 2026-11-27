package machine

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/panochko/csa-lab4/internal/isa"
)

// State описывает состояние после такта одной строкой журнала.
// sources — соответствие адреса инструкции фрагменту исходного кода.
func (m *Machine) State(executed int, sources map[uint32]string) string {
	f := m.nzvc
	flag := func(v bool, name string) string {
		if v {
			return name
		}
		return "-"
	}
	u := m.rom.code[executed]
	var b strings.Builder
	fmt.Fprintf(&b, "t=%-6d mPC=%02d IR=%08X PC=%04X AR=%04X DR=%08X BR=%08X", m.ticks, executed, m.ir, m.pc, m.ar, m.dr, m.br)
	fmt.Fprintf(&b, " R0=%08X R1=%08X SP=%05X FP=%05X", m.regs[isa.R0], m.regs[isa.R1], m.regs[isa.SP], m.regs[isa.FP])
	fmt.Fprintf(&b, " CNT=%d %s%s%s%s", m.cnt, flag(f.n, "N"), flag(f.z, "Z"), flag(f.v, "V"), flag(f.c, "C"))
	fmt.Fprintf(&b, " | %04X %s | %s", m.instrAddr, m.disassemble(m.instrAddr), u.note)
	if m.event != "" {
		fmt.Fprintf(&b, " | %s", m.event)
	}
	if src, ok := sources[m.instrAddr]; ok {
		fmt.Fprintf(&b, " ; %s", src)
	}
	return b.String()
}

// MicroPC возвращает адрес микрокоманды, которая выполнится следующей.
func (m *Machine) MicroPC() int { return m.mpc }

// disassemble читает память напрямую, не затрагивая кэш и такты.
func (m *Machine) disassemble(addr uint32) string {
	next := func() uint32 {
		if addr > isa.MemorySize-isa.WordSize {
			return 0
		}
		w := binary.LittleEndian.Uint32(m.cache.mem[addr:])
		addr += isa.WordSize
		return w
	}
	return isa.Decode(next).String()
}

// MicrocodeListing возвращает содержимое памяти микропрограмм.
func MicrocodeListing() string {
	r := buildROM()
	seq := [...]string{"next", "jump", "by opcode", "call src", "dst", "return", "cnt loop", "halt"}
	var b strings.Builder
	for i, u := range r.code {
		next := seq[u.seq]
		if u.seq == seqJump || u.seq == seqCntLoop {
			next = fmt.Sprintf("%s %02d", next, u.addr)
		}
		fmt.Fprintf(&b, "%02d  %-26s %s\n", i, u.note, next)
	}
	b.WriteString("\nopcode -> mPC:\n")
	for op := range isa.OpcodeCount {
		fmt.Fprintf(&b, "  %-5s %02d\n", op, r.opTable[op])
	}
	b.WriteString("\nmode -> mPC (src, dst):\n")
	modes := [...]string{"none", "reg", "imm", "abs", "ind", "based"}
	for _, md := range []isa.Mode{isa.Reg, isa.Imm, isa.Abs, isa.Ind, isa.Based} {
		dst := "-"
		if md == isa.Abs || md == isa.Ind || md == isa.Based {
			dst = fmt.Sprintf("%02d", r.dstTable[md])
		}
		fmt.Fprintf(&b, "  %-6s %02d  %s\n", modes[md], r.srcTable[md], dst)
	}
	return b.String()
}
