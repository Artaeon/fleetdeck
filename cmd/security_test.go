package cmd

import (
	"testing"
	"time"
)

func TestSecurityTimeoutBounds(t *testing.T) {
	for _, tc := range []struct {
		scan, total time.Duration
		valid       bool
	}{
		{5 * time.Minute, 30 * time.Minute, true},
		{15 * time.Minute, time.Hour, true},
		{time.Minute, 5 * time.Minute, true},
		{30 * time.Minute, 2 * time.Hour, true},
		{0, time.Hour, false},
		{-time.Minute, time.Hour, false},
		{31 * time.Minute, time.Hour, false},
		{time.Minute, 4 * time.Minute, false},
		{15 * time.Minute, 10 * time.Minute, false},
		{time.Minute, 2*time.Hour + time.Second, false},
	} {
		if err := validateSecurityTimeouts(tc.scan, tc.total); (err == nil) != tc.valid {
			t.Errorf("scan=%s total=%s: %v", tc.scan, tc.total, err)
		}
	}
}
