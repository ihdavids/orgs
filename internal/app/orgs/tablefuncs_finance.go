package orgs

import (
	"fmt"
	"math"
)

// The finance functions use the spreadsheet sign convention: money paid out
// is negative, money received positive. rate is per period, and type is 1
// when payments are due at the start of each period, 0 (the default) at the end.

// finArgs reads n required numbers and up to two optional ones that default to 0.
func finArgs(name string, args []interface{}, n int) ([]float64, error) {
	if len(args) < n || len(args) > n+2 {
		return nil, fmt.Errorf("%s expects %d to %d arguments got %d", name, n, n+2, len(args))
	}
	out := make([]float64, n+2)
	for i, a := range args {
		f, ok := cellFloat(a)
		if !ok {
			return nil, fmt.Errorf("%s: argument %d [%v] is not a number", name, i+1, toStr(a))
		}
		out[i] = f
	}
	return out, nil
}

// pmt(rate, nper, pv [, fv [, type]]) is the payment each period.
func tblPmt(args ...interface{}) (interface{}, error) {
	a, err := finArgs("pmt", args, 3)
	if err != nil {
		return nil, err
	}
	r, n, pv, fv, typ := a[0], a[1], a[2], a[3], a[4]
	if r == 0 {
		return -(pv + fv) / n, nil
	}
	g := math.Pow(1+r, n)
	return -r * (fv + pv*g) / ((1 + r*typ) * (g - 1)), nil
}

// pv(rate, nper, pmt [, fv [, type]]) is the present value.
func tblPv(args ...interface{}) (interface{}, error) {
	a, err := finArgs("pv", args, 3)
	if err != nil {
		return nil, err
	}
	r, n, pmt, fv, typ := a[0], a[1], a[2], a[3], a[4]
	if r == 0 {
		return -(fv + pmt*n), nil
	}
	g := math.Pow(1+r, n)
	return -(fv + pmt*(1+r*typ)*(g-1)/r) / g, nil
}

// fv(rate, nper, pmt [, pv [, type]]) is the future value.
func tblFv(args ...interface{}) (interface{}, error) {
	a, err := finArgs("fv", args, 3)
	if err != nil {
		return nil, err
	}
	r, n, pmt, pv, typ := a[0], a[1], a[2], a[3], a[4]
	if r == 0 {
		return -(pv + pmt*n), nil
	}
	g := math.Pow(1+r, n)
	return -(pv*g + pmt*(1+r*typ)*(g-1)/r), nil
}

// nper(rate, pmt, pv [, fv [, type]]) is the number of periods.
func tblNper(args ...interface{}) (interface{}, error) {
	a, err := finArgs("nper", args, 3)
	if err != nil {
		return nil, err
	}
	r, pmt, pv, fv, typ := a[0], a[1], a[2], a[3], a[4]
	if r == 0 {
		if pmt == 0 {
			return nil, fmt.Errorf("nper: payment of 0 at a rate of 0 never ends")
		}
		return -(pv + fv) / pmt, nil
	}
	p := pmt * (1 + r*typ)
	return math.Log((p-fv*r)/(p+pv*r)) / math.Log(1+r), nil
}

// npv(rate, values...) discounts values received at the end of periods 1, 2, ...
func tblNpv(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("npv expects (rate, values...)")
	}
	r, ok := cellFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("npv: rate is not a number")
	}
	acc := 0.0
	for i, v := range numbers(args[1:]...) {
		acc += v / math.Pow(1+r, float64(i+1))
	}
	return acc, nil
}

// irr(values...) is the rate at which the values, one per period starting
// now, have a net present value of 0.
func tblIrr(args ...interface{}) (interface{}, error) {
	vals := numbers(args...)
	npv := func(r float64) float64 {
		acc := 0.0
		for i, v := range vals {
			acc += v / math.Pow(1+r, float64(i))
		}
		return acc
	}
	// Bracket a sign change, then bisect: slower than Newton but it cannot
	// wander off on a flat or awkward curve.
	lo, hi := -0.9999, 1.0
	for npv(lo)*npv(hi) > 0 && hi < 1e6 {
		hi *= 2
	}
	if npv(lo)*npv(hi) > 0 {
		return nil, fmt.Errorf("irr: no rate makes these values sum to 0 (they need a sign change)")
	}
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if npv(lo)*npv(mid) <= 0 {
			hi = mid
		} else {
			lo = mid
		}
	}
	return (lo + hi) / 2, nil
}
