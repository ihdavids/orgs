package orgs

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/ihdavids/govaluate"
	"github.com/ihdavids/orgs/internal/common"
)

func (s *RangeIter) OpEq(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	switch v := left.(type) {
	case float64:
		if ri, ok := right.(*RangeIter); ok {
			return ri.GetCurFloat64() == v, nil
		}
	case time.Month:
		if ri, ok := right.(*RangeIter); ok {
			return int(ri.GetCurFloat64()) == int(v), nil
		}
	case time.Weekday:
		if ri, ok := right.(*RangeIter); ok {
			return int(ri.GetCurFloat64()) == int(v), nil
		}
	case int:
		if ri, ok := right.(*RangeIter); ok {
			return int(ri.GetCurFloat64()) == v, nil
		}
	case bool:
		if ri, ok := right.(*RangeIter); ok {
			return ri.GetCurBool() == v, nil
		}
	case string:
		if ri, ok := right.(*RangeIter); ok {
			return v == ri.EnsureFirst(), nil
		}
	case time.Time:
		if tm, ok := right.(time.Time); ok {
			return v == tm, nil
		}
	case common.OrgDuration:
		if tm, ok := right.(common.OrgDuration); ok {
			return v.Mins == tm.Mins, nil
		}

	case *RangeIter:
		switch r := right.(type) {
		case *RangeIter:
			return v.EnsureFirst() == r.EnsureFirst(), nil
		case float64:
			return v.GetCurFloat64() == r, nil
		case int:
			return int(v.GetCurFloat64()) == r, nil
		case bool:
			return v.GetCurBool() == r, nil
		case string:
			return v.EnsureFirst() == r, nil
		case time.Time:
			if tm, ok := left.(time.Time); ok {
				return r == tm, nil
			}
		case common.OrgDuration:
			if tm, ok := left.(common.OrgDuration); ok {
				return r.Mins == tm.Mins, nil
			}
		}
	}
	switch v := right.(type) {
	case float64:
		if ri, ok := left.(*RangeIter); ok {
			return ri.GetCurFloat64() == v, nil
		}
	case int:
		if ri, ok := left.(*RangeIter); ok {
			return int(ri.GetCurFloat64()) == v, nil
		}
	case time.Month:
		if ri, ok := left.(*RangeIter); ok {
			return int(ri.GetCurFloat64()) == int(v), nil
		}
	case time.Weekday:
		if ri, ok := left.(*RangeIter); ok {
			return int(ri.GetCurFloat64()) == int(v), nil
		}
	case time.Time:
		if ri, ok := left.(*RangeIter); ok {
			if tm, ok2 := ri.TestGetCurTime(); ok2 {
				return v == tm, nil
			}
		}
	case common.OrgDuration:
		if ri, ok := left.(*RangeIter); ok {
			if tm, ok2 := ri.TestGetCurDuration(); ok2 {
				return v.Mins == tm.Mins, nil
			}
		}

	case bool:
		if ri, ok := left.(*RangeIter); ok {
			return ri.GetCurBool() == v, nil
		}
	}
	return nil, fmt.Errorf("unknown range equals error, type mismatch? [%v] vs [%v]", left, right)
}

type NumberOp func(l, r float64) (interface{}, error)
type StringOp func(l, r string) (interface{}, error)
type BoolOp func(l, r bool) (interface{}, error)
type ExtensionOp func(l, r interface{}) (interface{}, error)

func (s *RangeIter) OpGen(name string, left interface{}, right interface{}, parameters govaluate.Parameters, numOp NumberOp, strOp StringOp, blOp BoolOp, ext ...ExtensionOp) (interface{}, error) {
	if left != nil {
		switch v := left.(type) {
		case float64:
			if ri, ok := right.(*RangeIter); ok {
				return numOp(v, ri.GetCurFloat64())
			}
		case int:
			if ri, ok := right.(*RangeIter); ok {
				r, e := numOp(float64(v), ri.GetCurFloat64())
				switch rval := r.(type) {
				case float64:
					return int(rval), e
				default:
					return rval, e
				}
			}
		case string:
			if ri, ok := right.(*RangeIter); ok {
				return strOp(v, ri.EnsureFirst())
			}
		case bool:
			if ri, ok := right.(*RangeIter); ok {
				return blOp(v, ri.GetCurBool())
			}
		case *RangeIter:
			switch r := right.(type) {
			case *RangeIter:
				le := v.NextVal()
				ri := r.NextVal()
				if a, ok := le.(float64); ok {
					if b, ok2 := ri.(float64); ok2 {
						return numOp(a, b)
					}
				}
				if a, ok := le.(string); ok {
					if b, ok2 := ri.(string); ok2 {
						return strOp(a, b)
					}
				}
				if a, ok := le.(bool); ok {
					if b, ok2 := ri.(bool); ok2 {
						return blOp(a, b)
					}
				}
				if len(ext) > 0 {
					if r, err := ext[0](le, ri); err == nil {
						return r, err
					}
				}
				return strOp(v.EnsureFirst(), r.EnsureFirst())
			case float64:
				return numOp(v.GetCurFloat64(), r)
			case int:
				x, e := numOp(v.GetCurFloat64(), float64(r))
				switch rval := x.(type) {
				case float64:
					return int(rval), e
				default:
					return rval, e
				}
			case bool:
				return blOp(v.GetCurBool(), r)
			case string:
				return strOp(v.EnsureFirst(), r)
			}
		}
	}
	switch v := right.(type) {
	case float64:
		// Unary operator!
		if left == nil {
			return numOp(0.0, v)
		}
		if ri, ok := left.(*RangeIter); ok {
			return numOp(ri.GetCurFloat64(), v)
		}

	case bool:
		// Unary operator!
		if left == nil {
			return blOp(false, v)
		}
		if ri, ok := left.(*RangeIter); ok {
			return blOp(ri.GetCurBool(), v)
		}
	case int:
		// Unary operator!
		if left == nil {
			r, e := numOp(0, float64(v))
			switch rval := r.(type) {
			case float64:
				return int(rval), e
			default:
				return rval, e
			}
		}
		if ri, ok := left.(*RangeIter); ok {
			r, e := numOp(ri.GetCurFloat64(), float64(v))
			switch rval := r.(type) {
			case float64:
				return int(rval), e
			default:
				return rval, e
			}
		}
	// This can ONLY happen in a unary operator sitation.
	// This means we only have a right side so we cannot
	// easily determine what to do with the right side.
	//
	// So we kind of have to guess...
	// It's not ideal but it's about the best we can do here.
	case *RangeIter:
		if a, ok := v.TestGetCurFloat64(); ok {
			return numOp(0.0, a)
		}
		if a, ok := v.TestGetCurBool(); ok {
			return blOp(false, a)
		}
		return strOp("", v.EnsureFirst())
	}
	if len(ext) > 0 {
		if r, err := ext[0](left, right); err == nil {
			return r, err
		}
	}

	if left != nil {
		return nil, fmt.Errorf("[%s] unknown generic operator error [%v](%s) [%s] [%v](%s)", name, left, reflect.TypeOf(left).Name, name, right, reflect.TypeOf(right).Name)
	} else if right != nil {
		return nil, fmt.Errorf("[%s] unknown generic operator error [%v](%s) [%s] [%v](%s)", name, left, "<nil>", name, right, reflect.TypeOf(right).Name)
	} else {
		return nil, fmt.Errorf("[%s] unknown generic operator error [%v](%s) [%s] [%v](%s)", name, left, "<nil>", name, right, "<nil>")
	}
}

func (s *RangeIter) OpAdd(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("+", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return l + r, nil
		},
		func(l, r string) (interface{}, error) {
			return l + r, nil
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("Add not implemented on bool")
		},
		func(l, r interface{}) (interface{}, error) {
			t1, islTime := l.(time.Time)
			t2, isrTime := r.(time.Time)
			d1, islDur := l.(common.OrgDuration)
			d2, isrDur := r.(common.OrgDuration)
			if islTime && isrDur {
				return t1.Add(d2.Duration()), nil
			}
			if isrTime && islDur {
				return t2.Add(d1.Duration()), nil
			}
			if islDur && isrDur {
				return common.NewDuration(d1.Mins + d2.Mins), nil
			}
			return 0, fmt.Errorf("add mismatched types")
		})
}
func (s *RangeIter) OpSub(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("-", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return l - r, nil
		},
		func(l, r string) (interface{}, error) {
			return "", fmt.Errorf("subtraction is not defined for strings")
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("subtraction not defined on bool")
		},
		func(l, r interface{}) (interface{}, error) {
			t1, islTime := l.(time.Time)
			t2, isrTime := r.(time.Time)
			d1, islDur := l.(common.OrgDuration)
			d2, isrDur := r.(common.OrgDuration)
			if islTime && isrDur {
				return t1.Add(-d2.Duration()), nil
			}
			if isrTime && islDur {

				return t2.Add(-d1.Duration()), nil
			}
			if islDur && isrDur {
				d3 := d1.Mins - d2.Mins
				if d3 < 0 {
					d3 = 0
				}
				return common.NewDuration(d3), nil
			}
			return 0, fmt.Errorf("sub mismatched types")
		})
}
func (s *RangeIter) OpGt(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen(">", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return l > r, nil
		},
		func(l, r string) (interface{}, error) {
			return strings.Compare(l, r) > 0, nil
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("gt not defined on bool")
		})
}
func (s *RangeIter) OpGte(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen(">=", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return l >= r, nil
		},
		func(l, r string) (interface{}, error) {
			return strings.Compare(l, r) >= 0, nil
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("gte not defined on bool")
		})
}
func (s *RangeIter) OpLt(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("<", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return l < r, nil
		},
		func(l, r string) (interface{}, error) {
			return strings.Compare(l, r) < 0, nil
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("lt not defined on bool")
		})
}
func (s *RangeIter) OpLte(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("<=", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return l <= r, nil
		},
		func(l, r string) (interface{}, error) {
			return strings.Compare(l, r) <= 0, nil
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("lte not defined on bool")
		})
}
func (s *RangeIter) OpMul(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("*", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return l * r, nil
		},
		func(l, r string) (interface{}, error) {
			return "", fmt.Errorf("multiply not defined on strings")
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("multiply not implemented on bool")
		})
}
func (s *RangeIter) OpDiv(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("/", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return l / r, nil
		},
		func(l, r string) (interface{}, error) {
			return "", fmt.Errorf("divide not defined on strings")
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("divide not implemented on bool")
		})
}

func (s *RangeIter) OpMod(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("%", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return math.Mod(l, r), nil
		},
		func(l, r string) (interface{}, error) {
			return "", fmt.Errorf("modulus not defined on strings")
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("modulus not implemented on bool")
		})
}

func (s *RangeIter) OpPow(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("**", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return math.Pow(l, r), nil
		},
		func(l, r string) (interface{}, error) {
			return "", fmt.Errorf("pow not defined on strings")
		},
		func(l, r bool) (interface{}, error) {
			return false, fmt.Errorf("pow not implemented on bool")
		})
}

func (s *RangeIter) OpAnd(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("&&", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return (l > 0) && (r > 0), nil
		},
		func(l, r string) (interface{}, error) {
			return !isEmpty(l) && !isEmpty(r), nil
		},
		func(l, r bool) (interface{}, error) {
			return l && r, nil
		})
}

func (s *RangeIter) OpOr(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("||", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return (l > 0) || (r > 0), nil
		},
		func(l, r string) (interface{}, error) {
			return !isEmpty(l) || !isEmpty(r), nil
		},
		func(l, r bool) (interface{}, error) {
			return l || r, nil
		})
}

// This is not and is a unary operator
func (s *RangeIter) OpInv(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	return s.OpGen("!", left, right, parameters,
		func(l, r float64) (interface{}, error) {
			return !(r > 0), nil
		},
		func(l, r string) (interface{}, error) {
			return isEmpty(r), nil
		},
		func(l, r bool) (interface{}, error) {
			return !r, nil
		})
}

func (s *RangeIter) OpNeg(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	switch v := left.(type) {
	case *RangeIter:
		if x, ok := v.TestGetCurFloat64(); ok {
			return -x, nil
		}
	}
	switch v := right.(type) {
	case *RangeIter:
		if x, ok := v.TestGetCurFloat64(); ok {
			return -x, nil
		}
	}
	return nil, fmt.Errorf("unknown range neg error")
}

func (s *RangeIter) OpNEq(left interface{}, right interface{}, parameters govaluate.Parameters) (interface{}, error) {
	if res, err := s.OpEq(left, right, parameters); err == nil {
		return !res.(bool), nil
	} else {
		return res, err
	}
}
