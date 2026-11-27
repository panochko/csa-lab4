package machine

import (
	"encoding/binary"
	"testing"

	"github.com/panochko/csa-lab4/internal/isa"
)

func op(m isa.Mode, reg uint8, ext uint32) isa.Operand {
	return isa.Operand{Mode: m, Reg: reg, Ext: ext}
}

// ticksOf измеряет такты одной инструкции без учёта ожидания памяти.
func ticksOf(t *testing.T, ins isa.Instruction) int {
	t.Helper()
	var image []byte
	for _, w := range ins.Encode() {
		image = binary.LittleEndian.AppendUint32(image, w)
	}
	m, err := New(image, []byte("x"), true)
	if err != nil {
		t.Fatal(err)
	}
	m.regs[isa.R1], m.regs[isa.R2], m.regs[isa.FP] = 256, 256, 512
	for {
		if err := m.Tick(); err != nil {
			t.Fatal(err)
		}
		if m.mpc == fetchAddr {
			s := m.Stats()
			// каждый такт ожидания посчитан в StallTicks
			return m.Ticks() - s.StallTicks
		}
	}
}

// Числа из таблицы тактов в README.
func TestInstructionTicks(t *testing.T) {
	r1, r2 := op(isa.Reg, isa.R1, 0), op(isa.Reg, isa.R2, 0)
	local := op(isa.Based, isa.FP, 0xFFFFFFFC)
	cases := []struct {
		ins  isa.Instruction
		want int
	}{
		{isa.Instruction{Op: isa.Add, Src: r1, Dst: r2}, 7},
		{isa.Instruction{Op: isa.Add, Src: local, Dst: r1}, 11},
		{isa.Instruction{Op: isa.Add, Src: r1, Dst: local}, 12},
		{isa.Instruction{Op: isa.Jmp, Src: op(isa.Imm, 0, 64)}, 8},
		{isa.Instruction{Op: isa.Addn, Dst: op(isa.Reg, isa.R0, 0), Args: []isa.Operand{r1, op(isa.Imm, 0, 5), local}}, 29},
	}
	for _, c := range cases {
		if got := ticksOf(t, c.ins); got != c.want {
			t.Errorf("%s: %d тактов, ожидалось %d", c.ins, got, c.want)
		}
	}
}

func TestCacheTiming(t *testing.T) {
	c := cache{enabled: true, mem: make([]byte, isa.MemorySize)}
	count := func() int {
		for n := 1; ; n++ {
			if _, ready, _ := c.read(0x40); ready {
				return n
			}
		}
	}
	if got := count(); got != 41 {
		t.Errorf("промах: %d тактов, ожидалось 41", got)
	}
	if got := count(); got != 1 {
		t.Errorf("попадание: %d тактов, ожидалось 1", got)
	}
	n := 1
	for ready, _ := c.write(0x40, 7); !ready; ready, _ = c.write(0x40, 7) {
		n++
	}
	if v, ready, _ := c.read(0x40); n != memoryDelay || !ready || v != 7 {
		t.Errorf("write-through: тактов %d, значение %d, ready %v", n, v, ready)
	}
}
