package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const testKey = "LENDCORE_TEST_CFG"

// unset guarantees testKey is absent; t.Setenv's cleanup restores the original state.
func unset(t *testing.T) {
	t.Helper()
	t.Setenv(testKey, "")
	assert.NoError(t, os.Unsetenv(testKey))
}

// assertPanicContains asserts f panics with an error whose message contains every part.
func assertPanicContains(t *testing.T, f func(), parts ...string) {
	t.Helper()
	r := func() (r any) {
		defer func() { r = recover() }()
		f()
		return nil
	}()
	err, ok := r.(error)
	if !assert.True(t, ok, "expected a panic with an error value") {
		return
	}
	for _, p := range parts {
		assert.ErrorContains(t, err, p)
	}
}

func TestString(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv(testKey, "hello")
		assert.Equal(t, "hello", String(testKey, "def"))
	})
	t.Run("unset uses default", func(t *testing.T) {
		unset(t)
		assert.Equal(t, "def", String(testKey, "def"))
	})
	t.Run("empty uses default", func(t *testing.T) {
		t.Setenv(testKey, "")
		assert.Equal(t, "def", String(testKey, "def"))
	})
}

func TestMustString(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv(testKey, "hello")
		assert.Equal(t, "hello", MustString(testKey))
	})
	t.Run("unset panics", func(t *testing.T) {
		unset(t)
		assertPanicContains(t, func() { MustString(testKey) }, testKey, "not set")
	})
	t.Run("empty panics", func(t *testing.T) {
		t.Setenv(testKey, "")
		assertPanicContains(t, func() { MustString(testKey) }, testKey, "not set")
	})
}

func TestInt(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv(testKey, "8080")
		assert.Equal(t, 8080, Int(testKey, 1))
	})
	t.Run("negative", func(t *testing.T) {
		t.Setenv(testKey, "-5")
		assert.Equal(t, -5, Int(testKey, 1))
	})
	t.Run("unset uses default", func(t *testing.T) {
		unset(t)
		assert.Equal(t, 42, Int(testKey, 42))
	})
	t.Run("empty uses default", func(t *testing.T) {
		t.Setenv(testKey, "")
		assert.Equal(t, 42, Int(testKey, 42))
	})
	for _, bad := range []string{"abc", "12.5", "1e3"} {
		t.Run("invalid "+bad+" panics", func(t *testing.T) {
			t.Setenv(testKey, bad)
			assertPanicContains(t, func() { Int(testKey, 1) }, testKey, "invalid int", bad)
		})
	}
}

func TestDuration(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv(testKey, "1500ms")
		assert.Equal(t, 1500*time.Millisecond, Duration(testKey, time.Second))
	})
	t.Run("hours", func(t *testing.T) {
		t.Setenv(testKey, "2h")
		assert.Equal(t, 2*time.Hour, Duration(testKey, time.Second))
	})
	t.Run("unset uses default", func(t *testing.T) {
		unset(t)
		assert.Equal(t, 3*time.Second, Duration(testKey, 3*time.Second))
	})
	t.Run("empty uses default", func(t *testing.T) {
		t.Setenv(testKey, "")
		assert.Equal(t, 3*time.Second, Duration(testKey, 3*time.Second))
	})
	for _, bad := range []string{"abc", "10"} {
		t.Run("invalid "+bad+" panics", func(t *testing.T) {
			t.Setenv(testKey, bad)
			assertPanicContains(t, func() { Duration(testKey, time.Second) }, testKey, "invalid duration", bad)
		})
	}
}
