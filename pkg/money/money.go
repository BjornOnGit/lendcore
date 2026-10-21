// Package money provides integer minor-unit money types. Floats are never used.
package money

import (
	"strconv"
	"strings"
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
