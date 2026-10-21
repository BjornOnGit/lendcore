package money

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMoneyString(t *testing.T) {
	tests := []struct {
		name   string
		amount Amount
		want   string
	}{
		{"zero", 0, "NGN 0.00"},
		{"one kobo", 1, "NGN 0.01"},
		{"99 kobo", 99, "NGN 0.99"},
		{"one naira", 100, "NGN 1.00"},
		{"thousands", 123450, "NGN 1,234.50"},
		{"millions", 100000000, "NGN 1,000,000.00"},
		{"negative one kobo", -1, "NGN -0.01"},
		{"negative naira and kobo", -150, "NGN -1.50"},
		{"negative thousands", -123450, "NGN -1,234.50"},
		{"max int64", math.MaxInt64, "NGN 92,233,720,368,547,758.07"},
		{"min int64", math.MinInt64, "NGN -92,233,720,368,547,758.08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Money{Amount: tt.amount, Currency: NGN}.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func ngn(a Amount) Money { return Money{Amount: a, Currency: NGN} }

func TestMoneyAdd(t *testing.T) {
	usd := Money{Amount: 1, Currency: "USD"}
	tests := []struct {
		name    string
		a, b    Money
		want    Money
		wantErr error
	}{
		{"normal", ngn(100), ngn(50), ngn(150), nil},
		{"zeros", ngn(0), ngn(0), ngn(0), nil},
		{"negative operand", ngn(-100), ngn(30), ngn(-70), nil},
		{"max plus zero", ngn(math.MaxInt64), ngn(0), ngn(math.MaxInt64), nil},
		{"reaches max exactly", ngn(math.MaxInt64 - 1), ngn(1), ngn(math.MaxInt64), nil},
		{"min plus max", ngn(math.MinInt64), ngn(math.MaxInt64), ngn(-1), nil},
		{"overflow above max", ngn(math.MaxInt64), ngn(1), Money{}, ErrOverflow},
		{"overflow below min", ngn(math.MinInt64), ngn(-1), Money{}, ErrOverflow},
		{"currency mismatch", ngn(100), usd, Money{}, ErrCurrencyMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Add(tt.b)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMoneySub(t *testing.T) {
	usd := Money{Amount: 1, Currency: "USD"}
	tests := []struct {
		name    string
		a, b    Money
		want    Money
		wantErr error
	}{
		{"normal", ngn(100), ngn(30), ngn(70), nil},
		{"negative result allowed", ngn(30), ngn(100), ngn(-70), nil},
		{"zeros", ngn(0), ngn(0), ngn(0), nil},
		{"minus negative", ngn(5), ngn(-5), ngn(10), nil},
		{"reaches max exactly", ngn(-1), ngn(math.MinInt64), ngn(math.MaxInt64), nil},
		{"overflow above max", ngn(math.MaxInt64), ngn(-1), Money{}, ErrOverflow},
		{"overflow below min", ngn(math.MinInt64), ngn(1), Money{}, ErrOverflow},
		{"zero minus min", ngn(0), ngn(math.MinInt64), Money{}, ErrOverflow},
		{"currency mismatch", ngn(100), usd, Money{}, ErrCurrencyMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Sub(tt.b)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAmountApplyBps(t *testing.T) {
	tests := []struct {
		name string
		a    Amount
		bps  int64
		want Amount
	}{
		{"15 percent of 10000", 10000, 1500, 1500},
		{"333 at 1 percent rounds down", 333, 100, 3},
		{"zero bps", 12345, 0, 0},
		{"zero amount", 0, 1500, 0},
		{"100 percent is identity", 12345, 10000, 12345},
		{"below half rounds down", 49, 100, 0},
		{"just below 1.5 rounds down", 149, 100, 1},
		{"tie 0.5 rounds up", 50, 100, 1},
		{"tie 1.5 rounds up", 150, 100, 2},
		{"tie 2.5 rounds up (not banker's)", 250, 100, 3},
		{"negative below half", -149, 100, -1},
		{"negative tie rounds toward +inf", -150, 100, -1},
		{"negative above half", -151, 100, -2},
		{"no intermediate overflow", math.MaxInt64, 10000, math.MaxInt64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.a.ApplyBps(tt.bps))
		})
	}
}

func TestAmountApplyBpsResultOverflowPanics(t *testing.T) {
	assert.Panics(t, func() { _ = Amount(math.MaxInt64).ApplyBps(20000) })
}

func TestAmountSplit(t *testing.T) {
	tests := []struct {
		name  string
		total Amount
		n     int
		want  []Amount
	}{
		{"one part", 100, 1, []Amount{100}},
		{"remainder on last", 100, 3, []Amount{33, 33, 34}},
		{"exact division", 99, 3, []Amount{33, 33, 33}},
		{"smaller than n", 1, 3, []Amount{0, 0, 1}},
		{"zero total", 0, 3, []Amount{0, 0, 0}},
		{"negative total", -10, 3, []Amount{-3, -3, -4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.total.Split(tt.n))
		})
	}
}

func TestAmountSplitInvalidNPanics(t *testing.T) {
	assert.Panics(t, func() { Amount(100).Split(0) })
	assert.Panics(t, func() { Amount(100).Split(-1) })
}

// For many totals and part counts: parts sum exactly to the total, all parts
// but the last are equal, and the last differs from them by exactly total % n.
func TestAmountSplitProperties(t *testing.T) {
	totals := []Amount{math.MinInt64, math.MinInt64 + 1, math.MaxInt64 - 1, math.MaxInt64}
	for a := Amount(-1000); a <= 1000; a++ {
		totals = append(totals, a)
	}
	for _, total := range totals {
		for n := 1; n <= 50; n++ {
			parts := total.Split(n)
			if len(parts) != n {
				t.Fatalf("Split(%d, %d): got %d parts", total, n, len(parts))
			}
			var sum Amount
			for _, p := range parts {
				sum += p
			}
			if sum != total {
				t.Fatalf("Split(%d, %d): parts sum to %d", total, n, sum)
			}
			for _, p := range parts[:n-1] {
				if p != parts[0] {
					t.Fatalf("Split(%d, %d): unequal leading parts %v", total, n, parts)
				}
			}
			if diff, rem := parts[n-1]-parts[0], total%Amount(n); diff != rem {
				t.Fatalf("Split(%d, %d): last differs by %d, want remainder %d", total, n, diff, rem)
			}
		}
	}
}
