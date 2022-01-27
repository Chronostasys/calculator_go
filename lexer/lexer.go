package lexer

import (
	"fmt"
	"log"
)

const (
	TYPE_INT       = 0
	TYPE_PLUS      = 1
	TYPE_SUB       = 2
	TYPE_MUL       = 3
	TYPE_DIV       = 4
	TYPE_LP        = 5  // "("
	TYPE_RP        = 6  // ")"
	TYPE_ASSIGN    = 7  // "="
	TYPE_RES_VAR   = 8  // "var"
	TYPE_RES_INT   = 9  // "int"
	TYPE_NL        = 10 // "\n"
	TYPE_VAR       = 11
	TYPE_FLOAT     = 12
	TYPE_RES_FLOAT = 13
)

var (
	input    string
	pos      int
	reserved = map[string]int{
		"var":   TYPE_RES_VAR,
		"int":   TYPE_RES_INT,
		"float": TYPE_RES_FLOAT,
	}
	ErrEOS  = fmt.Errorf("eos error")
	ErrTYPE = fmt.Errorf("the next token doesn't match the expected type")
)

func IsResType(token string) (code int, ok bool) {
	code, ok = reserved[token]
	return
}

func SetInput(s string) {
	pos = 0
	input = s
}

func peek() (ch rune, end bool) {
	if pos >= len(input) {
		return ch, true
	}
	ch = []rune(input)[pos]
	return ch, false
}

func getCh() (ch rune, end bool) {
	defer func() {
		pos++
	}()
	return peek()
}

func getChSkipEmpty() (ch rune, end bool) {
	ch, end = getCh()
	if end {
		return
	}
	if ch == ' ' || ch == '\t' {
		return getChSkipEmpty()
	}
	return
}
func isLetter(ch rune) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z')
}
func isLetterOrUnderscore(ch rune) bool {
	return isLetter(ch) || ch == '_'
}
func isNum(ch rune) bool {
	return '0' <= ch && ch <= '9'
}

func Retract(i int) {
	pos -= i
}

type Checkpoint struct {
	pos int
}

func SetCheckpoint() Checkpoint {
	return Checkpoint{
		pos: pos,
	}
}
func GobackTo(c Checkpoint) {
	pos = c.pos
}

func ScanType(code int) (token string, err error) {
	c, t, e := Scan()
	if c == code {
		return t, nil
	} else if e {
		return "", ErrEOS
	}
	pos -= len(t)
	return "", ErrTYPE
}

func Scan() (code int, token string, eos bool) {
	ch, end := getChSkipEmpty()
	if end {
		eos = end
		return
	}
	if isLetterOrUnderscore(ch) {
		i := []rune{ch}
		for {
			c, end := getCh()
			if end {
				break
			}
			if !isLetterOrUnderscore(c) && !isNum(c) {
				pos--
				break
			}
			i = append(i, c)
		}
		token = string(i)
		if tp, ok := reserved[token]; ok {
			return tp, token, end
		}
		return TYPE_VAR, string(i), end
	}
	if isNum(ch) {
		i := []rune{ch}
		t := TYPE_INT
		for {
			c, end := getCh()
			if end {
				break
			}
			if c == '.' {
				i = append(i, c)
				t = TYPE_FLOAT
				continue
			}
			if !isNum(c) {
				pos--
				break
			}
			i = append(i, c)
		}
		return t, string(i), end
	}
	switch ch {
	case '+':
		return TYPE_PLUS, "+", end
	case '-':
		return TYPE_SUB, "-", end
	case '*':
		return TYPE_MUL, "*", end
	case '/':
		return TYPE_DIV, "/", end
	case '(':
		return TYPE_LP, "(", end
	case ')':
		return TYPE_RP, ")", end
	case '=':
		return TYPE_ASSIGN, "=", end
	case '\n':
		return TYPE_NL, "\n", end
	case '\r':
		c, e := peek()
		if !e && c == '\n' {
			pos++
			return TYPE_NL, "\n", e
		}
	}
	log.Fatalf("unrecognized letter %c in pos %d", ch, pos)
	return

}
