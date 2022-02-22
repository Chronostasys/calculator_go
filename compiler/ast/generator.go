package ast

import (
	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/types"
	"github.com/llir/llvm/ir/value"
)

type ctx struct {
	idxmap []*ctx
	id     int
	i      int
	node   defNode
	father *ctx
}

func (c *ctx) setVals(st value.Value, s *Scope) {
	b := s.block
	for _, v := range c.idxmap {
		id := v.id
		if v.node != nil {
			v.node.setVal(func(*Scope) value.Value {
				return b.NewGetElementPtr(loadElmType(st.Type()), st,
					zero, constant.NewInt(types.I32, int64(id)))
			})
		}
		if v.idxmap != nil {
			subst := b.NewGetElementPtr(loadElmType(st.Type()), st,
				zero, constant.NewInt(types.I32, int64(id)))
			v.setVals(subst, s)
		}
	}
}

func buildCtx(sl *SLNode, s *Scope, tps []types.Type) ([]types.Type, *ctx) {
	defer func() {
		s.childrenScopes = nil
	}()
	c := &ctx{idxmap: []*ctx{}}
	var trf func(n Node)
	tpm := ir.NewModule()
	tpf := tpm.NewFunc("xxxx", types.Void)
	tpsc := newScope(tpf.NewBlock(""))
	tpsc.globalScope = tpsc
	tpsc.m = tpm

	for k, v := range s.globalScope.vartable {
		tpsc.vartable[k] = v
	}
	tpsc.Pkgname = s.Pkgname
	trf = func(n Node) {
		switch node := n.(type) {
		case *IfElseNode:
			ntps, ct := buildCtx(node.Statements.(*SLNode), s.addChildScope(tpf.NewBlock("")), []types.Type{})
			tps = append(tps, types.NewStruct(ntps...))
			ct.father = c
			ct.id = c.i
			c.idxmap = append(c.idxmap, ct)

			c.i++
		case *IfNode:
			ntps, ct := buildCtx(node.Statements.(*SLNode), s.addChildScope(tpf.NewBlock("")), []types.Type{})
			tps = append(tps, types.NewStruct(ntps...))
			ct.father = c
			ct.id = c.i
			c.idxmap = append(c.idxmap, ct)
			c.i++
		case *ForNode:
			ntps, ct := buildCtx(node.Statements.(*SLNode), s.addChildScope(tpf.NewBlock("")), []types.Type{})
			tps = append(tps, types.NewStruct(ntps...))
			ct.father = c
			ct.id = c.i
			c.idxmap = append(c.idxmap, ct)
			c.i++
		case *InlineFuncNode:
			ntps, ct := buildCtx(node.Body.(*SLNode), s, []types.Type{})
			tps = append(tps, types.NewStruct(ntps...))
			ct.father = c
			ct.i = c.i
			c.idxmap = append(c.idxmap, ct)
			c.i++
		case *DefineNode:
			tp, _ := node.TP.calc(s)
			tps = append(tps, tp)
			c.idxmap = append(c.idxmap, &ctx{id: c.i, father: c, node: node})

			c.i++
		case *DefAndAssignNode:
			var tp types.Type
			tp = node.calc(tpm, tpf, tpsc).Type()
			tps = append(tps, getElmType(tp))
			c.idxmap = append(c.idxmap, &ctx{id: c.i, father: c, node: node})
			c.i++
		default:

		}
	}
	tps = append(tps, types.NewPointer(types.NewFunc(types.Void)))
	c.idxmap = append(c.idxmap, &ctx{id: c.i, father: c})
	c.i++
	for _, v := range sl.Children {
		trf(v)
	}
	return tps, c
}

type YieldNode struct {
	Exp   Node
	label string
}

func (n *YieldNode) travel(f func(Node)) {
	f(n)
	if n.Exp != nil {
		n.Exp.travel(f)
	}
}

func (n *YieldNode) calc(m *ir.Module, f *ir.Func, s *Scope) value.Value {
	if n.Exp != nil {
		v := n.Exp.calc(m, f, s)
		v, err := implicitCast(loadIfVar(v, s), getElmType(s.yieldRet.Type()), s)
		if err != nil {
			panic(err)
		}
		store(v, s.yieldRet, s)
	}
	nb := f.NewBlock(n.label)
	store(constant.NewBlockAddress(f, nb), s.yieldBlock, s)
	s.block.NewRet(constant.True)
	s.block = nb
	return zero
}

type blockAddress struct {
	value.Value
}

func (c *blockAddress) IsConstant() {
}