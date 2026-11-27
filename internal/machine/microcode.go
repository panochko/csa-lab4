package machine

import "github.com/panochko/csa-lab4/internal/isa"

// Поля микроинструкции. Каждое поле управляет одним мультиплексором,
// защёлкой или устройством DataPath.

type aSel uint8 // мультиплексор левого входа АЛУ

const (
	aZero aSel = iota
	aReg
	aPC
	aDR
)

type bSel uint8 // мультиплексор правого входа АЛУ

const (
	bZero bSel = iota
	bBR
	bDR
	bFour
	bNeutral // нейтральный элемент: 0 для addn, 1 для muln
)

type aluSel uint8

const (
	aluAdd aluSel = iota
	aluSub
	aluOpcode // операция определяется опкодом в IR
)

type latch uint16 // какие регистры защёлкнуть в конце такта

const (
	lReg latch = 1 << iota
	lPC
	lPCCond // PC защёлкивается, если выполнено условие перехода
	lPCInc  // PC <- PC + 4 (отдельный инкрементор)
	lIR
	lAR
	lDR
	lBR
	lFlags
	lSpec
	lCntLoad
	lCntDec
)

type regSel uint8 // источник номера регистра

const (
	regSpec regSel = iota // SPEC[3:0]
	regSP
	regNary // IR.spec1[3:0] — приёмник addn/muln
)

type specSel uint8

const (
	specIR1 specSel = iota
	specIR2
	specDR // старший байт слова-описателя
)

type drSel uint8

const (
	drALU drSel = iota
	drMem
	drPort
)

type memSel uint8

const (
	memNone memSel = iota
	memRead
	memWrite
)

type seqSel uint8 // как выбирается следующий адрес микрокоманды

const (
	seqNext    seqSel = iota // mPC + 1
	seqJump                  // addr
	seqOpcode                // таблица опкодов
	seqSrc                   // вызов выборки источника, mRA <- mPC + 1
	seqDst                   // reg: mPC + 1; иначе вызов выборки приёмника, mRA <- mPC + 2
	seqReturn                // mRA
	seqCntLoop               // addr, пока CNT не станет 0
	seqHalt
)

type micro struct {
	a     aSel
	b     bSel
	alu   aluSel
	latch latch
	reg   regSel
	spec  specSel
	dr    drSel
	mem   memSel
	out   bool // вывод BR в порт
	seq   seqSel
	addr  int
	note  string
}

// microROM — память микропрограмм и таблицы диспетчеризации.
type microROM struct {
	code     []micro
	opTable  [isa.OpcodeCount]int
	srcTable [isa.ModeCount]int
	dstTable [isa.ModeCount]int
}

func (r *microROM) label() int { return len(r.code) }

func (r *microROM) emit(m ...micro) { r.code = append(r.code, m...) }

// Часто встречающиеся микрокоманды.
var (
	arFromPC  = micro{a: aPC, latch: lAR, note: "AR<-PC"}
	readNext  = micro{mem: memRead, dr: drMem, latch: lDR | lPCInc, note: "DR<-mem[AR], PC<-PC+4"}
	read      = micro{mem: memRead, dr: drMem, latch: lDR, note: "DR<-mem[AR]"}
	write     = micro{mem: memWrite, note: "mem[AR]<-DR"}
	arFromDR  = micro{b: bDR, latch: lAR, note: "AR<-DR"}
	arFromReg = micro{a: aReg, latch: lAR, note: "AR<-reg"}
	brFromDR  = micro{b: bDR, latch: lBR, note: "BR<-DR"}
	spDec     = micro{a: aReg, b: bFour, alu: aluSub, reg: regSP, latch: lReg, note: "SP<-SP-4"}
	spInc     = micro{a: aReg, b: bFour, reg: regSP, latch: lReg, note: "SP<-SP+4"}
	arFromSP  = micro{a: aReg, reg: regSP, latch: lAR, note: "AR<-SP"}
)

const fetchAddr = 0

// toFetch — последняя микрокоманда инструкции.
func toFetch(m micro) micro {
	m.seq, m.addr = seqJump, fetchAddr
	return m
}

// ret — последняя микрокоманда микроподпрограммы.
func ret(m micro) micro {
	m.seq = seqReturn
	return m
}

func loadSpec(s specSel, seq seqSel) micro {
	return micro{spec: s, latch: lSpec, seq: seq, note: "SPEC<-" + [...]string{"IR.spec1", "IR.spec2", "DR[31:24]"}[s]}
}

func buildROM() *microROM {
	r := &microROM{}

	// Выборка заголовка инструкции.
	r.emit(arFromPC, readNext, micro{b: bDR, latch: lIR, seq: seqOpcode, note: "IR<-DR"})

	r.buildOperandFetch()
	r.buildALU()
	r.buildNary()
	r.buildControlFlow()
	r.buildStackAndIO()
	return r
}

// Выборка источника кладёт значение в BR, выборка приёмника — адрес в AR.
func (r *microROM) buildOperandFetch() {
	r.srcTable[isa.Reg] = r.label()
	r.emit(ret(micro{a: aReg, latch: lBR, note: "BR<-reg"}))

	r.srcTable[isa.Imm] = r.label()
	r.emit(arFromPC, readNext, ret(brFromDR))

	r.srcTable[isa.Abs] = r.label()
	r.emit(arFromPC, readNext, arFromDR, read, ret(brFromDR))

	r.srcTable[isa.Ind] = r.label()
	r.emit(arFromReg, read, ret(brFromDR))

	based := micro{a: aReg, b: bDR, latch: lAR, note: "AR<-reg+DR"}
	r.srcTable[isa.Based] = r.label()
	r.emit(arFromPC, readNext, based, read, ret(brFromDR))

	r.dstTable[isa.Abs] = r.label()
	r.emit(arFromPC, readNext, ret(arFromDR))

	r.dstTable[isa.Ind] = r.label()
	r.emit(ret(arFromReg))

	r.dstTable[isa.Based] = r.label()
	r.emit(arFromPC, readNext, ret(based))
}

func (r *microROM) buildALU() {
	// Двухоперандные АЛУ-операции: одна микропрограмма на все опкоды.
	start := r.label()
	r.emit(
		loadSpec(specIR1, seqSrc),
		loadSpec(specIR2, seqDst),
		// приёмник — регистр
		micro{a: aReg, b: bBR, alu: aluOpcode, latch: lReg | lFlags, seq: seqJump, addr: fetchAddr, note: "reg<-reg op BR"},
		// приёмник — память
		read,
		micro{a: aDR, b: bBR, alu: aluOpcode, latch: lDR | lFlags, note: "DR<-DR op BR"},
		toFetch(write),
	)
	for _, op := range []isa.Opcode{isa.Add, isa.Adc, isa.Sub, isa.Sbc, isa.Mul, isa.Mulh, isa.Div, isa.Mod, isa.And, isa.Or, isa.Xor, isa.Shr} {
		r.opTable[op] = start
	}

	r.opTable[isa.Cmp] = r.label()
	r.emit(
		loadSpec(specIR1, seqSrc),
		loadSpec(specIR2, seqDst),
		micro{a: aReg, b: bBR, alu: aluSub, latch: lFlags, seq: seqJump, addr: fetchAddr, note: "NZVC<-reg-BR"},
		read,
		micro{a: aDR, b: bBR, alu: aluSub, latch: lFlags, seq: seqJump, addr: fetchAddr, note: "NZVC<-DR-BR"},
	)

	r.opTable[isa.Mov] = r.label()
	r.emit(loadSpec(specIR1, seqSrc))
	r.emitStoreBR()
}

// emitStoreBR — запись BR в приёмник (общая для mov, pop, in).
func (r *microROM) emitStoreBR() {
	r.emit(
		loadSpec(specIR2, seqDst),
		micro{b: bBR, latch: lReg, seq: seqJump, addr: fetchAddr, note: "reg<-BR"},
		micro{b: bBR, latch: lDR, note: "DR<-BR"},
		toFetch(write),
	)
}

func (r *microROM) buildNary() {
	start := r.label()
	r.opTable[isa.Addn], r.opTable[isa.Muln] = start, start
	r.emit(
		micro{latch: lCntLoad, note: "CNT<-IR.spec2"},
		micro{b: bNeutral, reg: regNary, latch: lReg, note: "rd<-neutral"},
	)
	loop := r.label()
	r.emit(
		arFromPC,
		readNext,
		loadSpec(specDR, seqSrc),
		micro{a: aReg, b: bBR, alu: aluOpcode, reg: regNary, latch: lReg | lFlags, note: "rd<-rd op BR"},
		// пока CNT не стал 0 — на loop, иначе на выборку следующей инструкции
		micro{latch: lCntDec, seq: seqCntLoop, addr: loop, note: "CNT<-CNT-1"},
	)
}

func (r *microROM) buildControlFlow() {
	jump := r.label()
	r.emit(
		loadSpec(specIR1, seqSrc),
		micro{b: bBR, latch: lPCCond, seq: seqJump, addr: fetchAddr, note: "if cond: PC<-BR"},
	)
	for op := isa.Jmp; op <= isa.Jc; op++ {
		r.opTable[op] = jump
	}

	r.opTable[isa.Call] = r.label()
	r.emit(
		loadSpec(specIR1, seqSrc),
		spDec,
		arFromSP,
		micro{a: aPC, latch: lDR, note: "DR<-PC"},
		write,
		micro{b: bBR, latch: lPC, seq: seqJump, addr: fetchAddr, note: "PC<-BR"},
	)

	r.opTable[isa.Ret] = r.label()
	r.emit(
		arFromSP,
		read,
		micro{b: bDR, latch: lPC, note: "PC<-DR"},
		toFetch(spInc),
	)

	r.opTable[isa.Halt] = r.label()
	r.emit(micro{seq: seqHalt, note: "halt"})
}

func (r *microROM) buildStackAndIO() {
	r.opTable[isa.Push] = r.label()
	r.emit(
		loadSpec(specIR1, seqSrc),
		spDec,
		arFromSP,
		micro{b: bBR, latch: lDR, note: "DR<-BR"},
		toFetch(write),
	)

	r.opTable[isa.Pop] = r.label()
	r.emit(arFromSP, read, brFromDR, spInc)
	r.emitStoreBR()

	r.opTable[isa.In] = r.label()
	r.emit(micro{dr: drPort, latch: lBR, note: "BR<-port"})
	r.emitStoreBR()

	r.opTable[isa.Out] = r.label()
	r.emit(
		loadSpec(specIR1, seqSrc),
		micro{out: true, seq: seqJump, addr: fetchAddr, note: "port<-BR"},
	)
}
