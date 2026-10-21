// Package config reads 12-factor configuration from environment variables.
//
// Failure policy: call these helpers once at process startup (typically in
// main). A missing required variable or an unparsable value panics with an
// error naming the variable, so a misconfigured service crashes immediately
// instead of running with a wrong value. An unset variable and a variable set
// to the empty string are treated the same: absent.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	return v, ok && v != ""
}

// String returns the value of key, or def if it is unset or empty.
func String(key, def string) string {
	if v, ok := lookup(key); ok {
		return v
	}
	return def
}

// MustString returns the value of key and panics if it is unset or empty.
func MustString(key string) string {
	v, ok := lookup(key)
	if !ok {
		panic(fmt.Errorf("config: required variable %s is not set", key))
	}
	return v
}

// Int returns key parsed as an int, or def if it is unset or empty.
// It panics if the value is not a valid integer.
func Int(key string, def int) int {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Errorf("config: %s: invalid int %q: %w", key, v, err))
	}
	return n
}

// Duration returns key parsed with time.ParseDuration (e.g. "1500ms", "2h"),
// or def if it is unset or empty. It panics if the value is not a valid duration.
func Duration(key string, def time.Duration) time.Duration {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Errorf("config: %s: invalid duration %q: %w", key, v, err))
	}
	return d
}
