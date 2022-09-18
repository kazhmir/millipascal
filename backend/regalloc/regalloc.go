package regalloc

import (
	"mpc/frontend/ast"
	ST "mpc/frontend/enums/symbolType"
	OT "mpc/frontend/enums/operandType"
	IT "mpc/frontend/enums/instrType"
	T "mpc/frontend/enums/Type"

	"strconv"
	"strings"
)

type value int
type reg int
type addr int

type stack struct {
	items []int
	top int
}

func (s *stack) String() string {
	output := []string{}
	for _, item := range s.items[:s.top+1] {
		output = append(output, strconv.Itoa(item))
	}
	return strings.Join(output, ", ")
}

func newStack(size int) *stack {
	items := make([]int, size)
	for i := range items {
		items[i] = size-i-1
	}
	return &stack{
		items: items,
		top: size-1,
	}
}

func (s *stack) HasItems() bool {
	return s.top >= 0
}

func (s *stack) Push(i int) {
	s.top++
	if s.top >= len(s.items) {
		s.items = append(s.items, make([]int, 2)...)
	}
	s.items[s.top] = i
}

func (s *stack) Pop() int {
	if s.top < 0 {
		return s.items[0]
	}
	item := s.items[s.top]
	s.top--
	return item
}

func (s *stack) Size() int {
	return s.top
}

type useInfo struct {
	IsReg bool
	Num int
	NextUse int
}

type deferredInstr struct {
	index int
	instr *ast.Instr
}

type state struct {
	AvailableRegs *stack
	// UsedRegs[ reg ] retuns the value stored in the register
	UsedRegs  map[reg]value

	AvailableAddr *stack
	// UsedAddr[ value ] retuns the value stored in the address
	UsedAddr  map[addr]value

	// LiveValues[ value ] retuns the register or address storing this value
	LiveValues  map[value]useInfo
	queuedInstr []deferredInstr
}

func newState(numRegs int) *state {
	return &state{
		AvailableRegs: newStack(numRegs),
		UsedRegs: map[reg]value{},

		AvailableAddr: newStack(16),
		UsedAddr: map[addr]value{},

		LiveValues: map[value]useInfo{},
	}
}

func (s *state) HasFreeRegs() bool {
	return s.AvailableRegs.HasItems()
}

func (s *state) Free(v value) {
	loc, ok := s.LiveValues[v]
	if !ok {
		panic("freeing unfound value")
	}
	delete(s.LiveValues, v)

	if loc.IsReg {
		r := reg(loc.Num)
		s.FreeReg(r)
	} else {
		a := addr(loc.Num)
		s.FreeAddr(a)
	}
}

func (s *state) FreeReg(r reg) {
	_, ok := s.UsedRegs[r]
	if ok {
		delete(s.UsedRegs, r)
		s.AvailableRegs.Push(int(r))
		return
	}
	panic("freeing unused register")
}

func (s *state) FreeAddr(a addr) {
	_, ok := s.UsedAddr[a]
	if ok {
		delete(s.UsedAddr, a)
		s.AvailableAddr.Push(int(a))
		return
	}
	panic("freeing unused addr")
}

func (s *state) AllocReg(v value) reg {
	r := reg(s.AvailableRegs.Pop())
	s.UsedRegs[r] = v
	s.LiveValues[v] = useInfo{IsReg: true, Num: int(r), NextUse: -1}
	return r
}

func (r *state) FurthestUse() reg {
	biggestIndex := 0
	bestReg:= reg(-1)
	for _, info := range r.LiveValues {
		if info.IsReg && info.NextUse > biggestIndex {
			biggestIndex = info.NextUse
			bestReg = reg(info.Num)
		}
	}

	return bestReg
}

func (s *state) Spill(r reg) addr {
	v, ok := s.UsedRegs[r]
	if !ok {
		panic("spilling unused register")
	}
	s.FreeReg(r)
	a := addr(s.AvailableAddr.Pop())
	s.LiveValues[v] = useInfo{IsReg: false, Num: int(a), NextUse: -1}
	return a
}

func Allocate(M *ast.Module, numRegs int) {
	for _, sy := range M.Globals {
		if sy.T == ST.Proc {
			allocProc(M, sy.Proc, numRegs)
		}
	}
}

func allocProc(M *ast.Module, p *ast.Proc, numRegs int) {
	var worklist = ast.FlattenGraph(p.Code)
	p.SpillRegionSize = 0
	for _, curr := range worklist {
		s := newState(numRegs)
		allocBlock(s, curr)
		top := s.AvailableAddr.Size()
		if int(top) > p.SpillRegionSize {
			p.SpillRegionSize = int(top)
		}
		insertQueuedInstrs(s, curr)
	}
}

func insertQueuedInstrs(s *state, bb *ast.BasicBlock) {
	for i, di := range s.queuedInstr {
		insertInstr(bb, di.index+i, di.instr)
	}
}

func allocBlock(s *state, bb *ast.BasicBlock) {
	for i, instr := range bb.Code {
		for opIndex, op := range instr.Operands {
			if op.T == OT.Temp {
				instr.Operands[opIndex] = ensure(s, bb, op, i)
				v := value(op.Num)
				freeIfNotNeeded(s, bb, v, i)
			}
		}
		for destIndex, op := range instr.Destination {
			if op.T == OT.Temp {
				instr.Destination[destIndex] = allocTemp(s, bb, op, i)
			}
		}
	}
}

func ensure(s *state, bb *ast.BasicBlock, op *ast.Operand, index int) *ast.Operand {
	v := value(op.Num)
	loc, ok := s.LiveValues[v]
	if !ok {
		return allocTemp(s, bb, op, index)
	}
	if loc.IsReg {
		r := reg(loc.Num)
		return newRegOp(r, op.Type)
	}
	a := addr(loc.Num)
	return loadSpill(s, bb, op, index, a)
}

func loadSpill(s *state, bb *ast.BasicBlock, op *ast.Operand, index int, a addr) *ast.Operand {
	rOp := allocTemp(s, bb, op, index)
	spillOp := newSpillOperand(a, op.Type)
	load := &ast.Instr{
		T: IT.LoadSpill,
		Type: op.Type,
		Operands: []*ast.Operand{spillOp},
		Destination: []*ast.Operand{rOp},
	}
	queueInstr(s, index-1, load)
	return rOp
}

func allocTemp(s *state, bb *ast.BasicBlock, op *ast.Operand, index int) *ast.Operand {
	if s.HasFreeRegs() {
		v := value(op.Num)
		r := s.AllocReg(v)
		return newRegOp(r, op.Type)
	}
	return spillRegister(s, bb, op, index)
}

func spillRegister(s *state, bb *ast.BasicBlock, op *ast.Operand, index int) *ast.Operand {
	calcNextUse(s, bb, op, index)
	r := s.FurthestUse()
	sNum := s.Spill(r)
	spillOp := newSpillOperand(sNum, op.Type)
	regOp := newRegOp(r, op.Type)
	store := &ast.Instr{
		T: IT.StoreSpill,
		Operands: []*ast.Operand{regOp},
		Destination: []*ast.Operand{spillOp},
	}
	queueInstr(s, index-1, store)

	v := value(op.Num)
	r2 := s.AllocReg(v)
	if r != r2 {
		panic("something went wrong")
	}
	return newRegOp(r, op.Type)
}

func newRegOp(r reg, t T.Type) *ast.Operand {
	return &ast.Operand{
		T: OT.Register,
		Num: int(r),
		Type: t,
	}
}

func newSpillOperand(sNum addr, t T.Type) *ast.Operand {
	return &ast.Operand {
		T: OT.Spill,
		Num: int(sNum),
		Type: t,
	}
}

func calcNextUse(s *state, bb *ast.BasicBlock, opTemp *ast.Operand, index int) {
	for v := range s.LiveValues {
		isNeeded(s, bb, v, index)
	}
}

func freeIfNotNeeded(s *state, bb *ast.BasicBlock, v value, index int) {
	if isNeeded(s, bb, v, index) {
		return
	}
	s.Free(v)
}

func isNeeded(s *state, bb *ast.BasicBlock, v value, index int) bool {
	if index + 1 >= len(bb.Code) {
		return false
	}
	useInfo, ok := s.LiveValues[v]
	if !ok {
		panic("isNeeded: value not found")
	}
	if useInfo.NextUse != -1 &&
		useInfo.NextUse > index {
		return true
	}
	for i, instr := range bb.Code[index+1:] {
		for _, op := range instr.Operands {
			if op.T == OT.Temp && op.Num == int(v) {
				useInfo.NextUse = i+index+1
				s.LiveValues[v] = useInfo
				return true
			}
		}
	}
	return false
}

func queueInstr(s *state, index int, instr *ast.Instr) {
	di := deferredInstr{index: index, instr: instr}
	s.queuedInstr = append(s.queuedInstr, di)
}

func insertInstr(bb *ast.BasicBlock, index int, instr *ast.Instr) {
	begin := bb.Code[:index+1]
	end := bb.Code[index:]
	bb.Code = append(begin, end...)
	bb.Code[index+1] = instr
}
