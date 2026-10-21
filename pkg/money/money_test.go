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
