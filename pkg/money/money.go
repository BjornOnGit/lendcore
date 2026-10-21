// Package money provides integer minor-unit money types. Floats are never used.
package money

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Sentinel errors returned by Money arithmetic.
var (
	// ErrCurrencyMismatch is returned when operands have different currencies.
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	// ErrOverflow is returned when a result does not fit in int64.
	ErrOverflow = errors.New("money: int64 overflow")
)

// Amount is a quantity of money in minor units (e.g. kobo for NGN).
type Amount int64

// Currency is an ISO 4217 currency code.
type Currency string

// NGN is the Nigerian naira.
const NGN Currency = "NGN"

// Money is an Amount tagged with its Currency.
type Money struct {
	Amount   Amount
	Currency Currency
}

// Add returns m + o. It fails on currency mismatch or int64 overflow.
func (m Money) Add(o Money) (Money, error) {
	if m.Currency != o.Currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, o.Currency)
	}
	if (o.Amount > 0 && m.Amount > math.MaxInt64-o.Amount) ||
		(o.Amount < 0 && m.Amount < math.MinInt64-o.Amount) {
		return Money{}, ErrOverflow
	}
	return Money{Amount: m.Amount + o.Amount, Currency: m.Currency}, nil
}

// Sub returns m - o. It fails on currency mismatch or int64 overflow.
// A negative result is allowed; callers enforce any non-negative rule.
func (m Money) Sub(o Money) (Money, error) {
	if m.Currency != o.Currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, o.Currency)
	}
	if (o.Amount < 0 && m.Amount > math.MaxInt64+o.Amount) ||
		(o.Amount > 0 && m.Amount < math.MinInt64+o.Amount) {
		return Money{}, ErrOverflow
	}
	return Money{Amount: m.Amount - o.Amount, Currency: m.Currency}, nil
}

const (
	bpsDenominator = 10_000 // 10,000 basis points = 100%
	bpsHalf        = 5_000
)

// ApplyBps returns a * bps / 10,000 rounded to the nearest minor unit.
//
// Rounding is round-half-up, i.e. floor(a*bps/10000 + 1/2): exact ties round
// toward positive infinity (2.5 -> 3, -2.5 -> -2). No floats are used and the
// intermediate product cannot overflow. It panics if the rounded result does
// not fit in int64, which is only reachable with bps above 10,000.
func (a Amount) ApplyBps(bps int64) Amount {
	p := new(big.Int).Mul(big.NewInt(int64(a)), big.NewInt(bps))
	p.Add(p, big.NewInt(bpsHalf))
	q := p.Div(p, big.NewInt(bpsDenominator)) // Euclidean division == floor for a positive divisor
	if !q.IsInt64() {
		panic("money: ApplyBps result overflows int64")
	}
	return Amount(q.Int64())
}

// Split divides a into n parts that sum exactly to a. Every part is a/n
// (truncated toward zero) except the last, which also takes the remainder a%n.
// It panics if n < 1.
func (a Amount) Split(n int) []Amount {
	if n < 1 {
		panic("money: Split requires n >= 1")
	}
	base, rem := a/Amount(n), a%Amount(n)
	parts := make([]Amount, n)
	for i := range parts {
		parts[i] = base
	}
	parts[n-1] += rem
	return parts
}

// String formats m in major units with thousands separators and two decimals,
// e.g. "NGN 1,234.50". Negative amounts render as "NGN -1,234.50".
func (m Money) String() string {
	digits := strconv.FormatInt(int64(m.Amount), 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	if len(digits) < 3 {
		digits = strings.Repeat("0", 3-len(digits)) + digits
	}
	major, minor := digits[:len(digits)-2], digits[len(digits)-2:]
	return string(m.Currency) + " " + sign + groupThousands(major) + "." + minor
}

// groupThousands inserts a comma every three digits, counting from the right.
func groupThousands(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	first := n % 3
	if first > 0 {
		b.WriteString(s[:first])
	}
	for i := first; i < n; i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
