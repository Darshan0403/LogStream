package storage

import (
	"testing"
	"time"
)

func TestLikePattern(t *testing.T) {
	cases := map[string]string{
		"connection refused": "%connection%refused%",
		"100% cpu":           `%100\%%cpu%`, // literal % escaped, space -> wildcard
		"a_b":                `%a\_b%`,
		`back\slash`:         `%back\\slash%`,
		"plain":              "%plain%",
	}
	for in, want := range cases {
		if got := likePattern(in); got != want {
			t.Errorf("likePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestISOWeekStart(t *testing.T) {
	// 2026-09-07 is a Monday.
	mon := time.Date(2026, 9, 7, 13, 30, 0, 0, time.UTC)
	if got := isoWeekStart(mon); !got.Equal(time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Monday: got %v", got)
	}
	// Sunday 2026-09-13 belongs to the week starting Monday 2026-09-07.
	sun := time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC)
	if got := isoWeekStart(sun); !got.Equal(time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Sunday: got %v", got)
	}
	// Wednesday 2026-09-09 -> same Monday.
	wed := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	if got := isoWeekStart(wed); !got.Equal(time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Wednesday: got %v", got)
	}
}
