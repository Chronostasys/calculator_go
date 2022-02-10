package parser

import (
	"github.com/Chronostasys/calculator_go/ast"
	"github.com/Chronostasys/calculator_go/lexer"
)

func (p *Parser) strExp() (n ast.Node, err error) {
	str, err := p.lexer.ScanType(lexer.TYPE_STR)
	if err != nil {
		return nil, err
	}
	_, err = p.lexer.ScanType(lexer.TYPE_PLUS)
	if err != nil {
		return nil, err
	}

	return &ast.StringNode{Str: str}, nil
}
