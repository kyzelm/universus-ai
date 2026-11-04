// Package sim is the deterministic core: 16.16 fixed-point, no floats, no math
// package, no clock, no I/O, single-threaded.
package sim

// Fix is a 16.16 fixed-point number: 16 integer bits, 16 fractional bits,
// stored in int32.
type Fix int32

const (
	FracBits = 16
	One      = Fix(1 << FracBits)
)

// Add and subtract with plain + and - — no helper needed.

func FromInt(i int) Fix { return Fix(i) << FracBits }

// ToInt floors, including for negatives: an arithmetic right shift makes
// Fix(-1).ToInt() == -1, not 0.
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
