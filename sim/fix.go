// Package sim is the deterministic core.
package sim

import (
	"fmt"
	"strings"
)

// Fix is a 16.16 fixed-point number.
type Fix int32

const (
	FracBits = 16
	One      = Fix(1 << FracBits)
)

// Add and subtract with plain + and - — no helper needed.

func FromInt(i int) Fix { return Fix(i) << FracBits }

// ToInt floors, including for negatives.
func (a Fix) ToInt() int { return int(a >> FracBits) }

// Mul is exact via an int64 intermediate.
func (a Fix) Mul(b Fix) Fix { return Fix((int64(a) * int64(b)) >> FracBits) }

// Div truncates toward zero when inexact.
func (a Fix) Div(b Fix) Fix { return Fix((int64(a) << FracBits) / int64(b)) }

func (a Fix) Abs() Fix {
	if a < 0 {
		return -a
	}
	return a
}

// UnmarshalJSON reads a decimal literal with integer arithmetic only (D68).
func (f *Fix) UnmarshalJSON(b []byte) error {
	s, neg := strings.CutPrefix(string(b), "-")
	whole, frac, dot := strings.Cut(s, ".")
	w, okW := digits(whole)
	d, okD := digits(frac[:min(len(frac), 9)])
	if !okW || !okD || whole == "" || len(whole) > 5 || (dot && frac == "") {
		return fmt.Errorf("sim: %s is not a 16.16 decimal", b)
	}
	den := int64(1)
	for range min(len(frac), 9) {
		den *= 10
	}
	// Round half away from zero: (2*num/den + 1) / 2.
	v := w<<FracBits + ((d<<(FracBits+1))/den+1)/2
	if neg {
		v = -v
	}
	if v > 1<<31-1 || v < -1<<31 {
		return fmt.Errorf("sim: %s is out of range for 16.16", b)
	}
	*f = Fix(v)
	return nil
}

// digits parses unsigned decimal digits; the empty string is 0.
func digits(s string) (int64, bool) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, true
}
