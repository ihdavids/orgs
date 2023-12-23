// https://github.com/bzick/tokenizer.git
// https://orgmode.org/manual/Matching-tags-and-properties.html
package orgs

import (
	"regexp"

	"github.com/bzick/tokenizer"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// define custom tokens keys
const (
	TEquality = 1
	TInclude  = 2
	TExclude  = 3
	TString   = 4
	TAnd      = 5
	TOr       = 6
	TAssign   = 7
	TRegex    = 8
)

type MatchExpr struct {
	Stream *tokenizer.Stream
	Parser *tokenizer.Tokenizer
	Expr   string
}

func (s *MatchExpr) Valid() bool {
	return s.Stream.IsValid()
}

func (s *MatchExpr) CurIs(key tokenizer.TokenKey) bool {
	return s.Stream.CurrentToken().Is(key)
}

func (s *MatchExpr) NextIs(key tokenizer.TokenKey) bool {
	return s.Stream.NextToken().Is(key)
}

func (s *MatchExpr) Next() {
	s.Stream.GoNext()
}

func (s *MatchExpr) Cur() *tokenizer.Token {
	return s.Stream.CurrentToken()
}

func (s *MatchExpr) PeekNext() *tokenizer.Token {
	return s.Stream.NextToken()
}

func opAnd(ok bool, res bool) bool {
	return ok && res
}

func opOr(ok bool, res bool) bool {
	return ok || res
}

func opInclude(v bool) bool {
	return v
}

func opExclude(v bool) bool {
	return !v
}

type NegateFunc func(bool) bool

type ResStack struct {
	s     []bool
	ok    bool
	op    int
	opInc NegateFunc
}

// IsEmpty: check if stack is empty
func (s *ResStack) IsEmpty() bool {
	return len(s.s) == 0
}

// Push a new value onto the stack
func (s *ResStack) Push(v bool) {
	s.s = append(s.s, v) // Simply append the new value to the end of the stack
}

// Remove and return top element of stack. Return false if stack is empty.
func (s *ResStack) Pop() bool {
	if s.IsEmpty() {
		return false
	} else {
		index := len(s.s) - 1   // Get the index of the top most element.
		element := (s.s)[index] // Index into the slice and obtain the element.
		s.s = (s.s)[:index]     // Remove it from the stack by slicing it off.
		return element
	}
}

func (s *ResStack) Op(val bool) {
	if s.op == TOr {
		s.Push(s.ok)
		s.ok = val
	} else {
		s.ok = s.ok && val
	}
	s.op = TOr
	s.opInc = opInclude
}

func (s *ResStack) SetOp(val int) {
	s.op = val
}

func (s *ResStack) SetNegate(f NegateFunc) {
	s.opInc = f
}

func (s *ResStack) Result() bool {
	for !s.IsEmpty() {
		s.ok = s.Pop() || s.ok
	}
	return s.ok
}

func TestEquality(op string, a int, b int) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	case "<":
		return a < b
	case ">":
		return a > b
	default:
		return false
	}
}

func CheckString(test string, tok *tokenizer.Token) bool {
	if tok.Key() == TString || tok.Key() == tokenizer.TokenString {
		tvalue := tok.ValueString()
		return tvalue == test
	}
	if tok.Key() == TRegex {
		tvalue := tok.ValueString()
		if ok, err := regexp.MatchString(test, tvalue); err == nil && ok {
			return true
		} else {
			// TODO: log an error
		}
	}
	// TODO: Log an error!
	return false
}

func (s *MatchExpr) EvalSection(ofile *common.OrgFile, sec *org.Section) bool {
	res := ResStack{ok: true, op: TAnd}

	// iterate over each token
	for s.Valid() {
		tok := s.Cur()
		//fmt.Printf("ID: %d\n", tok.ID())
		switch tok.Key() {
		case TInclude:
			res.SetNegate(opInclude)
			break
		case TExclude:
			res.SetNegate(opExclude)
			break
		case TAnd:
			res.SetOp(TAnd)
			res.SetNegate(opInclude)
			break
		case TOr:
			res.SetOp(TOr)
			res.SetNegate(opInclude)
			break
		case TRegex:
			// This is a tag check
			tstr := tok.ValueString()
			v := HasTagRegex(tstr, sec, ofile.Doc)
			res.Op(v)
		case tokenizer.TokenKeyword:
			tstr := tok.ValueString()
			if tstr == "SCHEDULED" {
				// TODO: Implement this!

			} else if tstr == "TODO" && s.PeekNext().Key() == TAssign {
				s.Next() // Skip equals
				s.Next() // Skip value check
				r := CheckString(sec.Headline.Status, s.Cur())
				res.Op(r)
			} else if tstr == "LEVEL" {
				s.Next()
				op := s.Cur()
				if op.Key() == TEquality {
					s.Next()
					v := s.Cur()
					if v.IsNumber() {
						val := int(v.ValueInt64())
						r := TestEquality(op.ValueString(), sec.Headline.Lvl, val)
						res.Op(r)
					} else {
						// TODO: Error about bad number
						return false
					}

				} else {
					// TODO: Error about bad comparison
					return false
				}
			} else if s.PeekNext().Key() == TAssign { // Property check
				if sec.Headline.Properties != nil {
					if v, o := sec.Headline.Properties.Get(tstr); o {
						s.Next() // Skip equals
						s.Next() // Skip value check
						tvalue := s.Cur().ValueString()
						res.Op(v == tvalue)
					}
				}
			} else { // Must be a tag
				v := HasTag(tstr, sec, ofile.Doc)
				res.Op(v)
			}
			break
		default:
			// TODO ERROR LOG
			return false
		}
		s.Next()
	}
	return res.Result()
}

func (s *MatchExpr) Reset() {
	s.Stream = s.Parser.ParseString(s.Expr)
}

func (s *MatchExpr) Close() {
	s.Stream.Close()
}

func NewMatchExpr(expr string) *MatchExpr {
	exp := &MatchExpr{}
	// configure tokenizer
	parser := tokenizer.New()
	parser.DefineTokens(TEquality, []string{"<", "<=", "==", ">=", ">", "!="})
	parser.DefineTokens(TAssign, []string{"="})
	parser.DefineTokens(TInclude, []string{"+"})
	parser.DefineTokens(TExclude, []string{"-"})
	parser.DefineTokens(TAnd, []string{"&"})
	parser.DefineTokens(TOr, []string{"|"})
	//parser.DefineTokens(TMath, []string{"+", "-", "/", "*", "%"})
	parser.DefineStringToken(TString, "{", "}").SetEscapeSymbol(tokenizer.BackSlash)
	parser.DefineStringToken(TRegex, "\"", "\"").SetEscapeSymbol(tokenizer.BackSlash)
	exp.Parser = parser
	exp.Expr = expr
	// create tokens stream
	exp.Stream = parser.ParseString(expr)
	return exp
}
