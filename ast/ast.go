package ast

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Chronostasys/calculator_go/lexer"
	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/enum"
	"github.com/llir/llvm/ir/types"
	"github.com/llir/llvm/ir/value"
)

var (
	globalScope = newScope(nil)
	typedic     = map[int]types.Type{
		lexer.TYPE_RES_FLOAT:   lexer.DefaultFloatType(),
		lexer.TYPE_RES_INT:     lexer.DefaultIntType(),
		lexer.TYPE_RES_BOOL:    types.I1,
		lexer.TYPE_RES_FLOAT32: types.Float,
		lexer.TYPE_RES_INT32:   types.I32,
		lexer.TYPE_RES_FLOAT64: types.Double,
		lexer.TYPE_RES_INT64:   types.I64,
		lexer.TYPE_RES_BYTE:    types.I8,
		lexer.TYPE_RES_VOID:    types.Void,
	}
)

type VNode interface {
	V() value.Value
}

type Node interface {
	calc(*ir.Module, *ir.Func, *scope) value.Value
}

func PrintTable() {
	fmt.Println(globalScope)
}

type BinNode struct {
	Op    int
	Left  Node
	Right Node
}

func loadIfVar(l value.Value, s *scope) value.Value {

	if t, ok := l.Type().(*types.PointerType); ok {
		return s.block.NewLoad(t.ElemType, l)
	}
	return l
}

func hasFloatType(b *ir.Block, ts ...value.Value) (bool, []value.Value) {
	for _, v := range ts {
		switch v.Type().(type) {
		case *types.FloatType:
		case *types.IntType:
			tp := v.Type().(*types.IntType)
			if tp.BitSize == 1 {
				return false, ts
			}
		default:
			return false, ts
		}
	}
	hasfloat := false
	var maxF *types.FloatType = types.Half
	var maxI *types.IntType = types.I8
	for _, v := range ts {
		t, ok := v.Type().(*types.FloatType)
		if ok {
			hasfloat = true
			if t.Kind > maxF.Kind {
				maxF = t
			}
		} else {
			tp := v.Type().(*types.IntType)
			if tp.BitSize > maxI.BitSize {
				maxI = tp
			}
		}
	}
	re := []value.Value{}
	for _, v := range ts {
		if hasfloat {
			t, ok := v.Type().(*types.FloatType)
			if ok {
				if t.Kind == maxF.Kind {
					re = append(re, v)
				} else {
					re = append(re, b.NewFPExt(v, maxF))
				}
			} else {
				re = append(re, b.NewSIToFP(v, maxF))
			}
		} else {
			t := v.Type().(*types.IntType)
			if t.BitSize == maxI.BitSize {
				re = append(re, v)
			} else {
				re = append(re, b.NewZExt(v, maxI))
			}
		}
	}

	return hasfloat, re
}

func (n *BinNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	rawL, rawR := n.Left.calc(m, f, s), n.Right.calc(m, f, s)
	l, r := loadIfVar(rawL, s), loadIfVar(rawR, s)
	hasF, re := hasFloatType(s.block, l, r)
	l, r = re[0], re[1]
	switch n.Op {
	case lexer.TYPE_PLUS:
		if hasF {
			return s.block.NewFAdd(l, r)
		}
		return s.block.NewAdd(l, r)
	case lexer.TYPE_DIV:
		if hasF {
			return s.block.NewFDiv(l, r)
		}
		return s.block.NewSDiv(l, r)
	case lexer.TYPE_MUL:
		if hasF {
			return s.block.NewFMul(l, r)
		}
		return s.block.NewMul(l, r)
	case lexer.TYPE_SUB:
		if hasF {
			return s.block.NewFSub(l, r)
		}
		return s.block.NewSub(l, r)
	case lexer.TYPE_ASSIGN:
		val := rawL
		r, err := implicitCast(r, l.Type(), s)
		if err != nil {
			panic(err)
		}
		store(r, val, s)
		switch n.Right.(type) {
		case *VarBlockNode, *TakePtrNode:
			getVarNode(n.Left).setHeap(getVarNode(n.Right).getHeap(s), s)
		case *TakeValNode:
			if strings.Contains(r.Type().String(), "*") {
				getVarNode(n.Left).setHeap(getVarNode(n.Right).getHeap(s), s)
			} else {
				getVarNode(n.Left).setHeap(false, s)
			}
		default:
			if all, ok := rawR.(*ir.InstAlloca); ok {
				getVarNode(n.Left).setHeap(mallocTable[all], s)
			} else {
				getVarNode(n.Left).setHeap(false, s)
			}
		}
		// if nd, ok := n.Right.(*VarBlockNode); ok {
		// 	getVarNode(n.Left).setHeap(nd.getHeap(s), s)
		// } else {
		// 	if all, ok := rawR.(*ir.InstAlloca); ok {
		// 		getVarNode(n.Left).setHeap(mallocTable[all], s)
		// 	}
		// 	getVarNode(n.Left).setHeap(false, s)
		// }
		return val
	default:
		panic("unexpected op")
	}
}

func getVarNode(n Node) alloca {

	for {
		if node, ok := n.(*TakeValNode); ok {
			n = node.Node
		} else if node, ok := n.(*TakePtrNode); ok {
			n = node.Node
		} else {
			a, _ := n.(alloca)
			return a
		}
	}
}

func store(r, lptr value.Value, s *scope) value.Value {
	if r.Type().Equal(lptr.Type().(*types.PointerType).ElemType) {
		s.block.NewStore(r, lptr)
		return lptr
	}
	if _, ok := lptr.Type().(*types.PointerType).ElemType.(*interf); ok {
		store := &ir.InstStore{Src: r, Dst: lptr}
		s.block.Insts = append(s.block.Insts, store)
		return lptr
	}

	panic("store failed")
}

type NumNode struct {
	Val value.Value
}

func (n *NumNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	return n.Val
}

type UnaryNode struct {
	Op    int
	Child Node
}

var zero = constant.NewInt(types.I32, 0)

func (n *UnaryNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	c := loadIfVar(n.Child.calc(m, f, s), s)
	switch n.Op {
	case lexer.TYPE_PLUS:
		return c
	case lexer.TYPE_SUB:
		hasF, re := hasFloatType(s.block, c)
		if hasF {
			return s.block.NewFSub(constant.NewFloat(c.Type().(*types.FloatType), 0), re[0])
		}
		return s.block.NewSub(zero, c)
	default:
		panic("unexpected op")
	}
}

func getElmType(v interface{}) types.Type {
	return reflect.Indirect(reflect.ValueOf(v).Elem()).FieldByName("ElemType").Interface().(types.Type)
}

func getTypeName(v interface{}) string {
	return reflect.Indirect(reflect.ValueOf(v).Elem()).FieldByName("ElemType").MethodByName("Name").Call([]reflect.Value{})[0].String()
}

type VarBlockNode struct {
	Token       string
	Idxs        []Node
	parent      value.Value
	Next        *VarBlockNode
	heap        *varheap
	allocOnHeap bool
}
type alloca interface {
	getHeap(s *scope) (onheap bool)
	setHeap(onheap bool, s *scope)
	setAlloc(onheap bool)
}

func (n *VarBlockNode) getHeap(s *scope) (onheap bool) {
	if n.parent == nil {
		// head node
		var err error
		var val *variable
		val, err = s.searchVar(n.Token)
		if err != nil {
			// TODO module
			panic(fmt.Errorf("variable %s not defined", n.Token))
		}
		return val.heap.onheap()
	}
	panic("this func shall only be called on root varblocknode")
}
func (n *VarBlockNode) setAlloc(onheap bool) {
	n.allocOnHeap = onheap
}
func (n *VarBlockNode) setHeap(onheap bool, s *scope) {
	if n.parent == nil {
		// head node
		var err error
		var val *variable
		val, err = s.searchVar(n.Token)
		if err != nil {
			// TODO module
			panic(fmt.Errorf("variable %s not defined", n.Token))
		}
		heap := val.heap
		for {
			if n.Next == nil {
				heap.heap = onheap
				break
			}
			n = n.Next
			if heap.innervar == nil {
				heap.innervar = map[string]*varheap{}
			}
			heap.innervar[n.Token] = &varheap{}
			heap = heap.innervar[n.Token]
		}

		return
	}
	panic("this func shall only be called on root varblocknode")
}

func (n *VarBlockNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	var va value.Value
	loadWraped := func() {
		if n.heap != nil && n.heap.heap {
			tp := va.Type()
			if elm, ok := tp.(*types.PointerType); ok {
				if _, ok := elm.ElemType.(*types.PointerType); !ok {
					va = s.block.NewPtrToInt(va, lexer.DefaultIntType())
					wrappertp := types.NewStruct(types.I8, elm.ElemType)
					va = s.block.NewIntToPtr(va, types.NewPointer(wrappertp))
					va = s.block.NewGetElementPtr(wrappertp, va,
						constant.NewIndex(zero),
						constant.NewIndex(constant.NewInt(types.I32, int64(1))))
				}
			}

		}
	}
	if n.parent == nil {
		// head node
		var err error
		var val *variable
		val, err = s.searchVar(n.Token)
		va = val.v
		if err != nil {
			// TODO module
			panic(fmt.Errorf("variable %s not defined", n.Token))
		}
		n.heap = val.heap
	} else {
		va = n.parent
		s1 := getTypeName(va.Type())
		tp := globalScope.getStruct(s1)
		fi := tp.fieldsIdx[n.Token]
		va = s.block.NewGetElementPtr(tp.structType, va,
			constant.NewIndex(zero),
			constant.NewIndex(constant.NewInt(types.I32, int64(fi.idx))))
	}
	idxs := n.Idxs
	if len(idxs) > 0 {
		// dereference the pointer
		va = deReference(va, s)
	}
	loadWraped()
	for _, v := range idxs {
		tp := getElmType(va.Type())
		idx := loadIfVar(v.calc(m, f, s), s)
		if _, ok := idx.Type().(*types.IntType); !ok {
			// TODO indexer reload
			panic("not impl")
		}
		va = s.block.NewGetElementPtr(tp, va,
			constant.NewIndex(zero),
			idx,
		)
	}
	if n.Next == nil {
		return va
	}

	// dereference the pointer
	va = deReference(va, s)
	n.Next.parent = va
	if n.heap != nil && n.heap.innervar != nil && n.heap.innervar[n.Next.Token] != nil {
		n.Next.heap = n.heap.innervar[n.Next.Token]
	}
	return n.Next.calc(m, f, s)
}

func deReference(va value.Value, s *scope) value.Value {
	tpptr := va.Type()
	for {
		if ptr, ok := tpptr.(*types.PointerType); ok {
			tpptr = ptr.ElemType
			if _, ok := tpptr.(*types.PointerType); ok {
				va = s.block.NewLoad(tpptr, va)
			} else {
				if inter, ok := tpptr.(*interf); ok {
					// interface type, return it's real type
					realTP := inter.innerType

					return s.block.NewIntToPtr(s.block.NewLoad(tpptr, va), types.NewPointer(realTP))
				}
				break
			}
		}
	}
	return va
}

// SLNode statement list node
type SLNode struct {
	Children []Node
}

type escNode struct {
	token    string
	initNode alloca
}

func (n *SLNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	var escMap = map[string][]*escNode{}
	var defMap = map[string]bool{}
	var escPoint = []string{}
	var heapAllocTable = map[string]bool{}
	for _, v := range n.Children {
		switch node := v.(type) {
		case *BinNode:
			if node.Op == lexer.TYPE_ASSIGN {
				left := getVarNode(node.Left).(*VarBlockNode)
				right := getVarNode(node.Right)
				if right == nil {
					continue
				}
				name := left.Token
				if !defMap[left.Token] {
					name = "extern.." + name
				}
				if r, ok := right.(*VarBlockNode); ok {
					rname := r.Token
					if !defMap[r.Token] {
						rname = "extern.." + rname
					}
					escMap[name] = append(escMap[name], &escNode{token: rname})
				} else {
					escMap[name] = append(escMap[name], &escNode{initNode: right})
				}
			}
		case *DefAndAssignNode:
			defMap[node.ID] = true
			name := node.ID
			right := getVarNode(node.Val)
			if right == nil {
				continue
			}
			if r, ok := right.(*VarBlockNode); ok {
				rname := r.Token
				if !defMap[r.Token] {
					rname = "extern.." + rname
				}
				escMap[name] = append(escMap[name], &escNode{token: rname})
			} else {
				escMap[name] = append(escMap[name], &escNode{initNode: right})
			}
		case *DefineNode:
			defMap[node.ID] = true
		case *RetNode:
			right := getVarNode(node.Exp)
			if right == nil {
				continue
			}
			if r, ok := right.(*VarBlockNode); ok {
				rname := r.Token
				if !defMap[r.Token] {
					rname = "extern.." + rname
				}
				escPoint = append(escPoint, rname)
			} else {
				right.setAlloc(true)
			}
		case *CallFuncNode:
			for _, v := range node.Params {
				right := getVarNode(v)
				if right == nil {
					continue
				}
				if r, ok := right.(*VarBlockNode); ok {
					rname := r.Token
					if !defMap[r.Token] {
						rname = "extern.." + rname
					}
					escPoint = append(escPoint, rname)
				} else {
					right.setAlloc(true)
				}
			}

		}
	}
	for _, v := range escPoint {
		if defMap[v] {
			heapAllocTable[v] = true
		}
		next := escMap[v]
		delete(escMap, v)

		findEsc(next, defMap, heapAllocTable, escMap)
	}
	if !strings.Contains(f.Ident(), "heapalloc") {
		s.heapAllocTable = heapAllocTable
	}
	for _, v := range n.Children {
		v.calc(m, f, s)
	}
	return zero
}
func findEsc(next []*escNode, defMap map[string]bool, heapAllocTable map[string]bool, escMap map[string][]*escNode) {
	if next == nil {
		return
	}
	for _, v := range next {
		if v.initNode == nil && defMap[v.token] {
			heapAllocTable[v.token] = true
			next = escMap[v.token]
			delete(escMap, v.token)
			findEsc(next, defMap, heapAllocTable, escMap)
		} else {
			v.initNode.setAlloc(true)
		}
	}
}

type ProgramNode struct {
	Children []Node
}

func (n *ProgramNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	n.Emit(m)
	return zero
}
func (n *ProgramNode) Emit(m *ir.Module) value.Value {
	// define all interfaces
	for _, v := range globalScope.interfaceDefFuncs {
		v()
	}

	// define all structs
	for {
		failed := []func(m *ir.Module) error{}
		for _, v := range globalScope.defFuncs {
			if v(m) != nil {
				failed = append(failed, v)
			}
		}
		globalScope.defFuncs = failed
		if len(failed) == 0 {
			break
		}
	}
	// add all func declaration to scope
	for _, v := range globalScope.funcDefFuncs {
		v()
	}

	for _, v := range n.Children {
		v.calc(m, nil, globalScope)
	}
	return zero
}

type EmptyNode struct {
}

func (n *EmptyNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	return zero
}

type DefineNode struct {
	ID  string
	TP  TypeNode
	Val value.Value
}

func (n *DefineNode) V() value.Value {
	return n.Val
}

func (n *DefineNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	tp, err := n.TP.calc(s)
	if err != nil {
		panic(err)
	}
	if f == nil {
		// TODO global
		n.Val = m.NewGlobal(n.ID, tp)
	} else {
		if s.heapAllocTable[n.ID] {
			gfn := globalScope.genericFuncs["heapalloc"]
			fnv := gfn(m, s, n.TP)
			n.Val = s.block.NewCall(fnv)
			s.addVar(n.ID, &variable{n.Val, &varheap{heap: true}})
		} else {
			n.Val = s.block.NewAlloca(tp)
			s.addVar(n.ID, &variable{n.Val, &varheap{}})
		}
	}
	return n.Val
}

type ParamNode struct {
	ID  string
	TP  TypeNode
	Val value.Value
}

func (n *ParamNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	tp, err := n.TP.calc(s)
	if err != nil {
		panic(err)
	}
	n.Val = ir.NewParam(n.ID, tp)
	return n.Val
}
func (n *ParamNode) V() value.Value {
	return n.Val
}

type ParamsNode struct {
	Params []*ParamNode
	Ext    bool
}

func (n *ParamsNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {

	return zero
}

type FuncNode struct {
	Params       *ParamsNode
	ID           string
	RetType      TypeNode
	Statements   Node
	Fn           *ir.Func
	DefaultBlock *ir.Block
	Generics     []string
}

func (n *FuncNode) AddtoScope() {
	if len(n.Generics) > 0 {
		globalScope.addGeneric(n.ID, func(m *ir.Module, s *scope, gens ...TypeNode) value.Value {
			sig := fmt.Sprintf("%s<", n.ID)
			for i, v := range n.Generics {
				tp, _ := gens[i].calc(s)
				s.genericMap[v] = tp
				if i != 0 {
					sig += ","
				}
				sig += tp.String()
			}
			sig += ">"
			fn, err := globalScope.searchVar(sig)
			if err == nil {
				return fn.v
			}
			psn := n.Params
			ps := []*ir.Param{}
			for _, v := range psn.Params {
				p := v
				tp, err := p.TP.calc(s)
				if err != nil {
					panic(err)
				}
				param := ir.NewParam(p.ID, tp)
				ps = append(ps, param)
			}
			tp, err := n.RetType.calc(s)
			if err != nil {
				panic(err)
			}
			fun := m.NewFunc(sig, tp, ps...)
			n.Fn = fun
			globalScope.addVar(sig, &variable{fun, &varheap{}})
			b := fun.NewBlock("")
			childScope := s.addChildScope(b)
			n.DefaultBlock = b
			for i, v := range ps {
				ptr := b.NewAlloca(v.Type())
				store(v, ptr, childScope)
				childScope.addVar(psn.Params[i].ID, &variable{ptr, &varheap{}})
			}
			n.Statements.calc(m, fun, childScope)
			return fun
		})
		return
	} else {
		globalScope.funcDefFuncs = append(globalScope.funcDefFuncs, func() {
			psn := n.Params
			ps := []*ir.Param{}
			for _, v := range psn.Params {
				p := v
				tp, err := p.TP.calc(globalScope)
				if err != nil {
					panic(err)
				}
				param := ir.NewParam(p.ID, tp)
				ps = append(ps, param)
			}
			tp, err := n.RetType.calc(globalScope)
			if err != nil {
				panic(err)
			}
			globalScope.addVar(n.ID, &variable{ir.NewFunc(n.ID, tp, ps...), &varheap{}})
		})
	}
}

func (n *FuncNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	if len(n.Generics) > 0 {
		// generic function will be generate while call
		return zero
	}
	psn := n.Params
	ps := []*ir.Param{}
	childScope := s.addChildScope(nil)
	for _, v := range psn.Params {
		param := v.calc(m, f, s).(*ir.Param)
		ps = append(ps, param)
	}
	tp, err := n.RetType.calc(s)
	if err != nil {
		panic(err)
	}
	fn := m.NewFunc(n.ID, tp, ps...)
	n.Fn = fn
	b := fn.NewBlock("")
	childScope.block = b

	n.DefaultBlock = b
	for i, v := range ps {
		ptr := b.NewAlloca(v.Type())
		store(v, ptr, childScope)
		childScope.addVar(psn.Params[i].ID, &variable{ptr, &varheap{}})
	}

	s.addVar(n.ID, &variable{n.Fn, &varheap{}})

	n.Statements.calc(m, fn, childScope)
	return fn
}

type CallFuncNode struct {
	Params   []Node
	FnNode   Node
	Generics []TypeNode
}

func (n *CallFuncNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	varNode := n.FnNode.(*VarBlockNode)
	fnNode := varNode
	prev := fnNode
	for {
		if fnNode.Next == nil {
			prev.Next = nil
			break
		}
		prev = fnNode
		fnNode = fnNode.Next
	}
	var fn *ir.Func

	params := []value.Value{}
	poff := 0
	if fnNode != varNode {
		alloca := deReference(varNode.calc(m, f, s), s)
		name := strings.Trim(alloca.Type().String(), "*%")
		name = name + "." + fnNode.Token
		var err error
		var fnv value.Value
		if len(n.Generics) > 0 {
			if gfn, ok := globalScope.genericFuncs[name]; ok {
				fnv = gfn(m, s, n.Generics...)
			} else {
				panic(fmt.Errorf("cannot find generic method %s", name))
			}
		} else {
			var va *variable
			va, err = s.searchVar(name)
			fnv = va.v
			if err != nil {
				panic(err)
			}
		}
		fn = fnv.(*ir.Func)
		if _, ok := fn.Sig.Params[0].(*types.PointerType); ok {
			alloca = deReference(alloca, s)
		} else {
			alloca = loadIfVar(alloca, s)
		}
		params = append(params, alloca)
		poff = 1
	} else {
		if len(n.Generics) > 0 {
			if gfn, ok := globalScope.genericFuncs[fnNode.Token]; ok {
				fn = gfn(m, s, n.Generics...).(*ir.Func)
			} else {
				panic(fmt.Errorf("cannot find generic method %s", fnNode.Token))
			}
		} else {
			fn = fnNode.calc(m, f, s).(*ir.Func)
		}
	}
	for i, v := range n.Params {
		tp := fn.Params[i+poff].Typ
		v2 := v.calc(m, f, s)
		v1 := loadIfVar(v2, s)
		p, err := implicitCast(v1, tp, s)
		if err != nil {
			panic(err)
		}
		params = append(params, p)
	}
	re := s.block.NewCall(fn, params...)
	if re.Type().Equal(types.Void) {
		return re
	}
	// autoAlloc()
	alloc := s.block.NewAlloca(re.Type())
	store(re, alloc, s)
	if fnNode.Token == "heapalloc" {
		mallocTable[alloc] = true
	}
	return alloc
}

var mallocTable = map[*ir.InstAlloca]bool{}

type RetNode struct {
	Exp Node
}

func (n *RetNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	if n.Exp == nil {
		s.block.NewRet(nil)
		return zero
	}
	ret := n.Exp.calc(m, f, s)
	v, err := implicitCast(loadIfVar(ret, s), f.Sig.RetType, s)
	if err != nil {
		panic(err)
	}
	s.block.NewRet(v)
	return zero
}

type BoolConstNode struct {
	Val bool
}

func (n *BoolConstNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	return constant.NewBool(n.Val)
}

type CompareNode struct {
	Op    int
	Left  Node
	Right Node
}
type e struct {
	IntE   enum.IPred
	FloatE enum.FPred
}

var comparedic = map[int]e{
	lexer.TYPE_EQ:  {enum.IPredEQ, enum.FPredOEQ},
	lexer.TYPE_NEQ: {enum.IPredNE, enum.FPredONE},
	lexer.TYPE_LG:  {enum.IPredSGT, enum.FPredOGT},
	lexer.TYPE_LEQ: {enum.IPredSGE, enum.FPredOGE},
	lexer.TYPE_SM:  {enum.IPredSLT, enum.FPredOLT},
	lexer.TYPE_SEQ: {enum.IPredSLE, enum.FPredOLE},
}

func (n *CompareNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	l, r := loadIfVar(n.Left.calc(m, f, s), s), loadIfVar(n.Right.calc(m, f, s), s)
	hasF, re := hasFloatType(s.block, l, r)
	l, r = re[0], re[1]
	if hasF {
		return s.block.NewFCmp(comparedic[n.Op].FloatE, l, r)
	} else {
		return s.block.NewICmp(comparedic[n.Op].IntE, l, r)
	}
}

type BoolExpNode struct {
	Op    int
	Left  Node
	Right Node
}

func (n *BoolExpNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	l, r := loadIfVar(n.Left.calc(m, f, s), s), loadIfVar(n.Right.calc(m, f, s), s)
	if n.Op == lexer.TYPE_AND {
		return s.block.NewAnd(l, r)
	} else {
		return s.block.NewOr(l, r)
	}
}

type NotNode struct {
	Bool Node
}

func (n *NotNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	return s.block.NewICmp(enum.IPredEQ, loadIfVar(n.Bool.calc(m, f, s), s), constant.False)
}

type IfNode struct {
	BoolExp    Node
	Statements Node
}

var blockID = 100

func (n *IfNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	blockID++
	tt := f.NewBlock(strconv.Itoa(blockID))
	n.Statements.calc(m, f, s.addChildScope(tt))
	blockID++
	end := f.NewBlock(strconv.Itoa(blockID))
	s.block.NewCondBr(n.BoolExp.calc(m, f, s), tt, end)
	s.block = end
	if tt.Term == nil {
		tt.NewBr(end)
	}
	if s.parent.block != nil {
		end.NewBr(s.parent.block)
	}

	return zero
}

type IfElseNode struct {
	BoolExp    Node
	Statements Node
	ElSt       Node
}

func (n *IfElseNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	blockID++
	tt := f.NewBlock(strconv.Itoa(blockID))
	blockID++
	tf := f.NewBlock(strconv.Itoa(blockID))
	blockID++
	end := f.NewBlock(strconv.Itoa(blockID))
	s.block.NewCondBr(n.BoolExp.calc(m, f, s), tt, tf)
	s.block = end
	n.Statements.calc(m, f, s.addChildScope(tt))
	n.ElSt.calc(m, f, s.addChildScope(tf))
	if tt.Term == nil {
		tt.NewBr(end)
	}
	if tf.Term == nil {
		tf.NewBr(end)
	}
	if s.parent.block != nil {
		end.NewBr(s.parent.block)
	}
	return zero
}

type DefAndAssignNode struct {
	Val Node
	ID  string
}

func autoAlloc(m *ir.Module, id string, gtp TypeNode, tp types.Type, s *scope) (v value.Value, heap bool) {
	if s.heapAllocTable[id] {
		gfn := globalScope.genericFuncs["heapalloc"]
		fnv := gfn(m, s, gtp)
		v = s.block.NewCall(fnv)
		heap = true
	} else {
		v = s.block.NewAlloca(tp)
	}
	return

}

func (n *DefAndAssignNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	if strings.Contains(n.ID, ".") {
		panic("unexpected '.'")
	}
	if f != nil {
		rawval := n.Val.calc(m, f, s)
		val := loadIfVar(rawval, s)
		var v value.Value
		var heap bool
		var tp types.Type
		switch val.Type().(type) {
		case *types.FloatType:
			tp = lexer.DefaultFloatType()
			v, heap = autoAlloc(m, n.ID,
				&BasicTypeNode{ResType: lexer.TYPE_RES_FLOAT},
				tp, s)

		case *types.IntType:
			if val.Type().(*types.IntType).BitSize == 1 {
				tp = val.Type()
				v, heap = autoAlloc(m, n.ID,
					&BasicTypeNode{ResType: lexer.TYPE_RES_BOOL},
					tp, s)
			} else {
				tp = lexer.DefaultIntType()
				v, heap = autoAlloc(m, n.ID,
					&BasicTypeNode{ResType: lexer.TYPE_RES_INT},
					tp, s)
			}
		default:
			tp = val.Type()
			v, heap = autoAlloc(m, n.ID,
				&calcedTypeNode{tp},
				tp, s)
		}
		val, err := implicitCast(val, tp, s)
		if err != nil {
			panic(err)
		}
		va := &variable{v, &varheap{heap: heap}}
		store(val, v, s)
		if heap {
			s.addVar(n.ID, va)
			return v
		}
		switch n.Val.(type) {
		case *VarBlockNode, *TakePtrNode:
			va.heap = &varheap{heap: getVarNode(n.Val).getHeap(s)}
		case *TakeValNode:
			if strings.Contains(val.Type().String(), "*") {
				va.heap = &varheap{heap: getVarNode(n.Val).getHeap(s)}
			} else {
				va.heap = &varheap{heap: false}
			}
		default:
			if all, ok := rawval.(*ir.InstAlloca); ok {
				va.heap = &varheap{heap: mallocTable[all]}
			} else {
				va.heap = &varheap{heap: false}
			}
		}
		s.addVar(n.ID, va)
		return v
	}
	// TODO
	panic("not impl")
}

type ForNode struct {
	Bool         Node
	DefineAssign Node
	Assign       Node
	Statements   Node
}

func (n *ForNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	blockID++
	cond := f.NewBlock(strconv.Itoa(blockID))
	blockID++
	body := f.NewBlock(strconv.Itoa(blockID))
	blockID++
	end := f.NewBlock(strconv.Itoa(blockID))
	s.continueBlock = cond
	s.breakBlock = end
	child := s.addChildScope(body)
	condScope := s.addChildScope(cond)
	name := ""
	if n.DefineAssign != nil {
		n.DefineAssign.calc(m, f, s)
		name = n.DefineAssign.(*DefAndAssignNode).ID
	}
	if n.Bool != nil {
		s.block.NewCondBr(loadIfVar(n.Bool.calc(m, f, s), s), body, end)
	} else {
		s.block.NewBr(body)
	}
	s.block = end
	n.Statements.calc(m, f, child)
	if n.Assign != nil {
		n.Assign.calc(m, f, condScope)
	}
	if n.Bool != nil {
		cond.NewCondBr(loadIfVar(n.Bool.calc(m, f, condScope), condScope), body, end)
	} else {
		cond.NewBr(body)
	}
	child.block.NewBr(cond)
	if n.DefineAssign != nil {
		// a trick, ensure loop var cannot be use out of loop
		child.vartable[name] = s.vartable[name]
		delete(s.vartable, name)
	}
	return zero
}

type BreakNode struct {
}

func (n *BreakNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	if s.breakBlock == nil {
		panic("cannot break out of loop")
	}
	s.block.NewBr(s.breakBlock)
	return zero
}

type ContinueNode struct {
}

func (n *ContinueNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	if s.continueBlock == nil {
		panic("cannot continue out of loop")
	}
	s.block.NewBr(s.continueBlock)
	return zero
}

type structDefNode struct {
	id     string
	fields map[string]TypeNode
}

func NewStructDefNode(id string, fieldsMap map[string]TypeNode) Node {
	n := &structDefNode{id: id, fields: fieldsMap}
	defFunc := func(m *ir.Module) error {
		fields := []types.Type{}
		fieldsIdx := map[string]*field{}
		i := 0
		for k, v := range n.fields {
			tp, err := v.calc(globalScope)
			if err != nil {
				return err
			}
			fields = append(fields, tp)
			fieldsIdx[k] = &field{
				idx:   i,
				ftype: fields[i],
			}
			i++
		}
		globalScope.addStruct(n.id, &typedef{
			fieldsIdx:  fieldsIdx,
			structType: m.NewTypeDef(n.id, types.NewStruct(fields...)),
		})
		return nil
	}
	globalScope.defFuncs = append(globalScope.defFuncs, defFunc)
	return n

}

func (n *structDefNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	return zero
}

type StructInitNode struct {
	ID          []string
	Fields      map[string]Node
	allocOnHeap bool
}

func implicitCast(v value.Value, target types.Type, s *scope) (value.Value, error) {
	if v.Type().Equal(target) {
		return v, nil
	}
	if t, ok := target.(*interf); ok {
		if v.Type().Equal(t.IntType) {
			return v, nil
		}
	}
	switch v.Type().(type) {
	case *types.FloatType:
		tp := v.Type().(*types.FloatType)
		targetTp := target.(*types.FloatType)
		if targetTp.Kind < tp.Kind {
			return nil, fmt.Errorf("failed to perform impliciot cast from %T to %v", v, target)
		}
		return s.block.NewFPExt(v, targetTp), nil
	case *types.IntType:
		tp := v.Type().(*types.IntType)
		targetTp := target.(*types.IntType)
		if targetTp.BitSize < tp.BitSize {
			return nil, fmt.Errorf("failed to perform impliciot cast from %T to %v", v, target)
		}
		return s.block.NewZExt(v, targetTp), nil
	case *types.PointerType:
		v = deReference(v, s)
		tp, ok := target.(*interf)
		src := strings.Trim(v.Type().String(), "%*")
		if ok { // turn to interface
			for k, v1 := range tp.interfaceFuncs {
				fnv, err := s.searchVar(src + "." + k)
				if err != nil {
					goto FAIL
				}
				fn := fnv.v.(*ir.Func)
				for i, u := range v1.Params.Params {
					ptp, err := u.TP.calc(s)
					if err != nil {
						goto FAIL
					}
					if !fn.Sig.Params[i+1].Equal(ptp) {
						goto FAIL
					}
				}
				rtp, err := v1.RetType.calc(s)
				if err != nil {
					goto FAIL
				}
				if !fn.Sig.RetType.Equal(rtp) {
					goto FAIL
				}
			}
			// cast
			inst := s.block.NewPtrToInt(v, lexer.DefaultIntType())
			tp.innerType = v.Type().(*types.PointerType).ElemType
			return inst, nil
		}
	FAIL:
		return nil, fmt.Errorf("failed to cast %v to interface %v", v, target.Name())
	default:
		return nil, fmt.Errorf("failed to cast %v to %v", v, target)
	}
}

func (n *StructInitNode) setHeap(onheap bool, s *scope) {
	panic("not setable")
}
func (n *StructInitNode) getHeap(s *scope) (onheap bool) {
	return false
}
func (n *StructInitNode) setAlloc(onheap bool) {
	n.allocOnHeap = onheap
}
func (n *StructInitNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	if len(n.ID) > 1 {
		panic("not impl yet")
	} else {
		tp := globalScope.getStruct(n.ID[0])
		var alloca value.Value
		if n.allocOnHeap {
			gfn := globalScope.genericFuncs["heapalloc"]
			fnv := gfn(m, s, &BasicTypeNode{CustomTp: n.ID})
			alloca = s.block.NewCall(fnv)
		} else {
			alloca = s.block.NewAlloca(tp.structType)
		}

		var va value.Value = alloca
		if n.allocOnHeap {
			// unwrap
			va = s.block.NewPtrToInt(alloca, lexer.DefaultIntType())
			wrappertp := types.NewStruct(types.I8, tp.structType)
			va = s.block.NewIntToPtr(va, types.NewPointer(wrappertp))
			va = s.block.NewGetElementPtr(wrappertp, va,
				constant.NewIndex(zero),
				constant.NewIndex(constant.NewInt(types.I32, int64(1))))
		}

		// assign
		for k, v := range n.Fields {
			fi := tp.fieldsIdx[k]
			ptr := s.block.NewGetElementPtr(tp.structType, va,
				constant.NewIndex(zero),
				constant.NewIndex(constant.NewInt(types.I32, int64(fi.idx))))
			va, err := implicitCast(loadIfVar(v.calc(m, f, s), s), fi.ftype, s)
			if err != nil {
				panic(err)
			}
			store(va, ptr, s)
		}
		return alloca
	}
}

type BasicTypeNode struct {
	ResType  int
	CustomTp []string
	PtrLevel int
}

type TypeNode interface {
	calc(*scope) (types.Type, error)
	SetPtrLevel(int)
	String() string
}

type calcedTypeNode struct {
	tp types.Type
}

func (v *calcedTypeNode) SetPtrLevel(i int) {
	panic("not impl")
}
func (v *calcedTypeNode) calc(*scope) (types.Type, error) {
	return v.tp, nil
}
func (v *calcedTypeNode) String() string {
	panic("not impl")
}

type ArrayTypeNode struct {
	Len      int
	ElmType  TypeNode
	PtrLevel int
}

func (v *ArrayTypeNode) SetPtrLevel(i int) {
	v.PtrLevel = i
}
func (v *BasicTypeNode) SetPtrLevel(i int) {
	v.PtrLevel = i
}
func (v *ArrayTypeNode) String() string {
	t, err := v.calc(globalScope)
	if err != nil {
		panic(err)
	}
	tp := strings.Trim(t.String(), "%*")
	return tp
}
func (v *BasicTypeNode) String() string {
	t, err := v.calc(globalScope)
	if err != nil {
		panic(err)
	}
	tp := strings.Trim(t.String(), "%*")
	return tp
}
func (v *ArrayTypeNode) calc(s *scope) (types.Type, error) {
	elm, err := v.ElmType.calc(s)
	if err != nil {
		return nil, err
	}
	var tp types.Type
	tp = types.NewArray(uint64(v.Len), elm)
	for i := 0; i < v.PtrLevel; i++ {
		tp = types.NewPointer(tp)
	}
	return tp, nil
}

func (v *BasicTypeNode) calc(sc *scope) (types.Type, error) {
	var s types.Type
	if len(v.CustomTp) == 0 {
		s = typedic[v.ResType]
	} else {
		if len(v.CustomTp) == 1 {
			st := types.NewStruct()
			def := sc.getStruct(v.CustomTp[0])
			if def != nil && def.interf {
				s = &interf{
					IntType:        lexer.DefaultIntType(),
					interfaceFuncs: def.funcs,
					name:           v.CustomTp[0],
				}

			} else if sc.getGenericType(v.CustomTp[0]) != nil {
				s = sc.getGenericType(v.CustomTp[0])
			} else {
				st.TypeName = v.CustomTp[0]
				s = st
			}
		} else {
			panic("not impl")
		}
	}
	if s == nil {
		return nil, errVarNotFound
	}
	for i := 0; i < v.PtrLevel; i++ {
		s = types.NewPointer(s)
	}
	return s, nil
}

type ArrayInitNode struct {
	Type        TypeNode
	Vals        []Node
	allocOnHeap bool
}

func (n *ArrayInitNode) setAlloc(onheap bool) {
	n.allocOnHeap = onheap
}

func (n *ArrayInitNode) setHeap(onheap bool, s *scope) {
	panic("not setable")
}
func (n *ArrayInitNode) getHeap(s *scope) (onheap bool) {
	return false
}
func (n *ArrayInitNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	tp := n.Type
	atype, err := tp.calc(s)
	if err != nil {
		panic(err)
	}
	var alloca value.Value
	if n.allocOnHeap {
		gfn := globalScope.genericFuncs["heapalloc"]
		fnv := gfn(m, s, n.Type)
		alloca = s.block.NewCall(fnv)
	} else {
		alloca = s.block.NewAlloca(atype)
	}
	var va value.Value = alloca
	if n.allocOnHeap {
		// unwrap
		va = s.block.NewPtrToInt(alloca, lexer.DefaultIntType())
		wrappertp := types.NewStruct(types.I8, atype)
		va = s.block.NewIntToPtr(va, types.NewPointer(wrappertp))
		va = s.block.NewGetElementPtr(wrappertp, va,
			constant.NewIndex(zero),
			constant.NewIndex(constant.NewInt(types.I32, int64(1))))
	}
	for k, v := range n.Vals {
		ptr := s.block.NewGetElementPtr(atype, va,
			constant.NewIndex(zero),
			constant.NewIndex(constant.NewInt(types.I32, int64(k))))
		cs, err := implicitCast(loadIfVar(v.calc(m, f, s), s), atype, s)
		if err != nil {
			panic(err)
		}
		store(cs, ptr, s)
	}
	return alloca
}

type TakePtrNode struct {
	Node Node
}

func (n *TakePtrNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	v := n.Node.calc(m, f, s)
	ptr := s.block.NewAlloca(v.Type())
	s.block.NewStore(v, ptr)
	return ptr
}

type TakeValNode struct {
	Level int
	Node  Node
}

func (n *TakeValNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	v := n.Node.calc(m, f, s)

	for i := 0; i < n.Level; i++ {
		v = s.block.NewLoad(getElmType(v.Type()), v)
	}
	return v

}

type interfaceDefNode struct {
	id    string
	funcs map[string]*FuncNode
}

func NewSInterfaceDefNode(id string, funcsMap map[string]*FuncNode) Node {
	n := &interfaceDefNode{id: id, funcs: funcsMap}
	defFunc := func() {
		globalScope.addStruct(n.id, &typedef{
			interf: true,
			funcs:  funcsMap,
		})
	}
	globalScope.interfaceDefFuncs = append(globalScope.interfaceDefFuncs, defFunc)
	return n

}

func (n *interfaceDefNode) calc(m *ir.Module, f *ir.Func, s *scope) value.Value {
	return zero
}
