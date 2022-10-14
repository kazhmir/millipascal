package hirchecker

import (
	T "mpc/frontend/enums/Type"
	FT "mpc/frontend/enums/flowType"
	IT "mpc/frontend/enums/instrType"
	ST "mpc/frontend/enums/symbolType"
	hirc "mpc/frontend/enums/HIRClass"
	"mpc/frontend/errors"
	"mpc/frontend/ir"
	eu "mpc/frontend/util/errors"

	"strconv"
)

func Check(M *ir.Module) *errors.CompilerError {
	s := newState(M)
	for _, sy := range M.Globals {
		if sy.T == ST.Proc {
			s.proc = sy.Proc
			err := checkCode(s, sy.Proc.Code)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type state struct {
	m    *ir.Module
	proc *ir.Proc
	bb   *ir.BasicBlock
}

func newState(M *ir.Module) *state {
	return &state{
		m:         M,
	}
}

func checkCode(s *state, bb *ir.BasicBlock) *errors.CompilerError {
	if bb.Checked {
		return nil
	}
	s.bb = bb
	for _, instr := range bb.Code {
		err := checkInstr(s, instr)
		if err != nil {
			return err
		}
	}
	bb.Checked = true
	return checkJump(s)
}

func checkJump(s *state) *errors.CompilerError {
	bb := s.bb
	switch bb.Out.T {
	case FT.Jmp:
		return checkCode(s, bb.Out.True)
	case FT.If:
		err := checkCode(s, bb.Out.True)
		if err != nil {
			return err
		}
		return checkCode(s, bb.Out.False)
	case FT.Return:
		return checkRet(s, bb.Out.V)
	}
	return nil
}

func checkRet(s *state, rets []*ir.Operand) *errors.CompilerError {
	if len(s.proc.Rets) != len(rets) {
		has := strconv.Itoa(len(rets))
		wants := strconv.Itoa(len(s.proc.Rets))
		return eu.NewInternalSemanticError("invalid number of returns: has "+ has + " wanted " +wants )
	}
	for i, wanted_ret := range s.proc.Rets {
		curr_ret := rets[i]
		if wanted_ret != curr_ret.Type {
			has := curr_ret.Type.String()
			wants := wanted_ret.String()
			return eu.NewInternalSemanticError("invalid return for procedure: has "+has+" wanted "+wants)
		}
	}
	return nil
}

type Checker struct {
	Class func(hirc.HIRClass)bool
	Type  func(T.Type)bool
}

func (c *Checker) Check(op *ir.Operand) bool {
	return c.Type(op.Type) && c.Class(op.HirC)
}

var any_oper = Checker {
	Class: hirc.IsOperable,
	Type: T.IsAny,
}

var any_res = Checker {
	Class: hirc.IsResult,
	Type: T.IsAny,
}

var num_oper = Checker {
	Class: hirc.IsOperable,
	Type: T.IsNumber,
}

var num_res = Checker {
	Class: hirc.IsResult,
	Type: T.IsNumber,
}

var bool_oper = Checker {
	Class: hirc.IsOperable,
	Type: T.IsBool,
}

var bool_res = Checker {
	Class: hirc.IsResult,
	Type: T.IsBool,
}

var nonPtr_oper = Checker {
	Class: hirc.IsOperable,
	Type: T.IsNonPtr,
}

var nonPtr_res = Checker {
	Class: hirc.IsResult,
	Type: T.IsNonPtr,
}

var ptr_oper = Checker {
	Class: hirc.IsOperable,
	Type: T.IsPtr,
}

var ptr_res = Checker {
	Class: hirc.IsResult,
	Type: T.IsPtr,
}

func checkInstr(s *state, instr *ir.Instr) *errors.CompilerError {
	switch instr.T {
	case IT.Add, IT.Sub, IT.Div, IT.Mult, IT.Rem:
		return checkArith(instr)
	case IT.Eq, IT.Diff, IT.Less, IT.More, IT.LessEq, IT.MoreEq:
		return checkComp(instr)
	case IT.Or, IT.And:
		return checkLogical(instr)
	case IT.Not:
		return checkNot(instr)
	case IT.UnaryMinus, IT.UnaryPlus:
		return checkUnaryArith(instr)
	case IT.Convert:
		return checkConvert(instr)
	case IT.Offset:
		return checkOffset(instr)
	case IT.LoadPtr:
		return checkLoadPtr(instr)
	case IT.StorePtr:
		return checkStorePtr(instr)
	case IT.Store:
		return checkStore(s, instr)
	case IT.Load:
		return checkLoad(s, instr)
	case IT.Call:
		return checkCall(s, instr)
	}
	panic("sumthin' went wong")
}

func checkArith(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 2, true)
	if err != nil {
		return err
	}
	a := instr.Operands[0]
	b := instr.Operands[1]
	dest := instr.Destination[0]
	err = checkEqual(instr, instr.Type, a.Type, b.Type, dest.Type)
	if err != nil {
		return err
	}
	return checkBinary(instr, num_oper, num_oper, num_res)
}

func checkComp(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 2, true)
	if err != nil {
		return err
	}
	a := instr.Operands[0]
	b := instr.Operands[1]
	err = checkEqual(instr, instr.Type, a.Type, b.Type)
	if err != nil {
		return err
	}
	return checkBinary(instr, any_oper, any_oper, bool_res)
}

func checkLogical(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 2, true)
	if err != nil {
		return err
	}
	a := instr.Operands[0]
	b := instr.Operands[1]
	dest := instr.Destination[0]
	err = checkEqual(instr, instr.Type, a.Type, b.Type, dest.Type)
	if err != nil {
		return err
	}
	return checkBinary(instr, bool_oper, bool_oper, bool_res)
}

func checkUnaryArith(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 1, true)
	if err != nil {
		return err
	}
	a := instr.Operands[0]
	dest := instr.Destination[0]
	err = checkEqual(instr, instr.Type, a.Type, dest.Type)
	if err != nil {
		return err
	}
	return checkUnary(instr, num_oper, num_res)
}

func checkNot(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 1, true)
	if err != nil {
		return err
	}
	a := instr.Operands[0]
	dest := instr.Destination[0]
	err = checkEqual(instr, instr.Type, a.Type, dest.Type)
	if err != nil {
		return err
	}
	return checkUnary(instr, bool_oper, bool_res)
}

func checkConvert(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 1, true)
	if err != nil {
		return err
	}
	dest := instr.Destination[0]
	err = checkEqual(instr, instr.Type, dest.Type) 
	if err != nil {
		return err
	}
	return checkUnary(instr, nonPtr_oper, nonPtr_res)
}

func checkOffset(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 2, true)
	if err != nil {
		return err
	}
	b := instr.Operands[1]
	err = checkEqual(instr, instr.Type, b.Type)
	if err != nil {
		return err
	}
	return checkBinary(instr, ptr_oper, num_oper, ptr_res)
}

func checkLoadPtr(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 1, true)
	if err != nil {
		return err
	}
	dest := instr.Destination[0]
	err = checkEqual(instr, instr.Type, dest.Type) 
	if err != nil {
		return err
	}
	return checkUnary(instr, ptr_oper, any_res)
}

func checkStorePtr(instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 1, true)
	if err != nil {
		return err
	}
	a := instr.Operands[0]
	err = checkEqual(instr, instr.Type, a.Type) 
	if err != nil {
		return err
	}
	return checkUnary(instr, any_oper, ptr_oper)
}

func checkLoad(s *state, instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 1, true)
	if err != nil {
		return err
	}
	a := instr.Operands[0]
	dest := instr.Destination[0]
	err = checkEqual(instr, instr.Type, a.Type, dest.Type)
	if err != nil {
		return err
	}
	err = checkUnary(instr, any_oper, any_res)
	if err != nil {
		return err
	}

	return nil
}

func checkStore(s *state, instr *ir.Instr) *errors.CompilerError {
	err := checkForm(instr, 1, true)
	if err != nil {
		return err
	}
	a := instr.Operands[0]
	dest := instr.Destination[0]
	err = checkEqual(instr, instr.Type, a.Type, dest.Type)
	if err != nil {
		return err
	}
	err = checkUnary(instr, any_oper, any_res)
	if err != nil {
		return err
	}

	return nil
}

func checkCall(s *state, instr *ir.Instr) *errors.CompilerError {
	if len(instr.Operands) == 0 {
		return malformedInstr(instr)
	}
	procOp := instr.Operands[0]
	if procOp.Symbol == nil {
		return procNotFound(instr)
	}
	proc := procOp.Symbol.Proc

	if len(instr.Operands) - 1 != len(proc.Args) {
		return procInvalidNumOfArgs(instr, proc)
	}
	if len(instr.Destination) != len(proc.Rets) {
		return procInvalidNumOfRets(instr, proc)
	}

	realArgs := instr.Operands[1:]

	for i, formal_arg := range proc.Args {
		real_arg := realArgs[i]
		if formal_arg.Type != real_arg.Type {
			return procBadArg(instr, formal_arg, real_arg)
		}
	}

	for i, formal_ret := range proc.Rets {
		real_ret := instr.Destination[i]
		if real_ret.Type != formal_ret {
			return procBadRet(instr, formal_ret, real_ret)
		}
	}
	return nil
}

func checkEqual(instr *ir.Instr, types ...T.Type) *errors.CompilerError {
	if len(types) == 0 {
		return nil
	}
	first := types[0]
	for _, t := range types[1:] {
		if first != t {
			return malformedEqualTypes(instr)
		}
	}
	return nil
}

func checkBinary(instr *ir.Instr, checkA, checkB, checkC Checker) *errors.CompilerError {
	a := instr.Operands[0]
	b := instr.Operands[1]
	dest := instr.Destination[0]

	if checkA.Check(a) &&
		checkB.Check(b) &&
		checkC.Check(dest){
		return nil
	}
	return malformedTypeOrClass(instr)
}

func checkUnary(instr *ir.Instr, checkA, checkC Checker) *errors.CompilerError {
	a := instr.Operands[0]
	dest := instr.Destination[0]

	if checkA.Check(a) &&
		checkC.Check(dest) {
		return nil
	}
	return malformedTypeOrClass(instr)
}

func checkForm(instr *ir.Instr, numOperands int, hasDest bool) *errors.CompilerError {
	if len(instr.Operands) != numOperands {
		return malformedInstr(instr)
	}
	for _, op := range instr.Operands {
		if op == nil {
			return malformedInstr(instr)
		}
	}
	if hasDest && instr.Destination == nil {
		return malformedInstr(instr)
	}
	return nil
}

func malformedInstr(instr *ir.Instr) *errors.CompilerError {
	return eu.NewInternalSemanticError("malformed instruction: " + instr.String())
}
func malformedEqualTypes(instr *ir.Instr) *errors.CompilerError {
	return eu.NewInternalSemanticError("unequal types: " + instr.String())
}
func malformedTypeOrClass(instr *ir.Instr) *errors.CompilerError {
	return eu.NewInternalSemanticError("malformed type or class: " + instr.String())
}
func procNotFound(instr *ir.Instr) *errors.CompilerError {
	return eu.NewInternalSemanticError("procedure not found: " + instr.String())
}
func procArgNotFound(instr *ir.Instr, d *ir.Symbol) *errors.CompilerError {
	return eu.NewInternalSemanticError("argument "+d.Name+" not found in: " + instr.String())
}
func procInvalidNumOfArgs(instr *ir.Instr, p *ir.Proc) *errors.CompilerError {
	n := strconv.Itoa(len(p.Args))
	beepBop := strconv.Itoa(len(instr.Operands) -1)
	return eu.NewInternalSemanticError("expected "+n+" arguments, instead found: " + beepBop)
}
func procInvalidNumOfRets(instr *ir.Instr, p *ir.Proc) *errors.CompilerError {
	n := strconv.Itoa(len(p.Rets))
	beepBop := strconv.Itoa(len(instr.Destination))
	return eu.NewInternalSemanticError("expected "+n+" returns, instead found: " + beepBop)
}
func procBadArg(instr *ir.Instr, d *ir.Symbol, op *ir.Operand) *errors.CompilerError {
	return eu.NewInternalSemanticError("argument "+op.String()+" doesn't match formal parameter "+d.Name+" in: " + instr.String())
}
func procBadRet(instr *ir.Instr, d T.Type, op *ir.Operand) *errors.CompilerError {
	return eu.NewInternalSemanticError("return "+op.String()+" doesn't match formal return "+d.String()+" in: " + instr.String())
}
