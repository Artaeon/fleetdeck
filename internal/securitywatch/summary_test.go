package securitywatch

import (
	"testing"
	"time"
)

func TestObservationHistoryIsBoundedAndDoesNotRepeatRetries(t *testing.T) {
	s := State{Version: 1}
	now := time.Now()
	for i := 0; i < 100; i++ {
		r := Report{CheckedAt: now.Add(time.Duration(i) * time.Hour), Host: Check{Status: "unknown"}}
		s.Record(r)
		s.Record(r)
	}
	if len(s.History) != 56 || !s.History[0].CheckedAt.Equal(now.Add(44*time.Hour)) || s.History[55].Unknown != 1 {
		t.Fatal(s.History)
	}
}
