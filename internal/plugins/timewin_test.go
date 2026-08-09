package plugins

import (
	"testing"
	"time"
)

func TestInWindowWrapsMidnight(t *testing.T) {
	from, ok1 := ParseHHMM("23:00")
	to, ok2 := ParseHHMM("07:00")
	if !ok1 || !ok2 {
		t.Fatal("parse")
	}
	if !InWindow(23*60+30, from, to) {
		t.Fatal("23:30 should be quiet")
	}
	if !InWindow(30, from, to) {
		t.Fatal("00:30 should be quiet")
	}
	if InWindow(12*60, from, to) {
		t.Fatal("12:00 should not be quiet")
	}
	if !InWindow(18*60, 18*60, 23*60) {
		t.Fatal("18:00 start inclusive")
	}
	if InWindow(23*60, 18*60, 23*60) {
		t.Fatal("23:00 end exclusive")
	}
}

func TestParseHHMM(t *testing.T) {
	m, ok := ParseHHMM("7:05")
	if !ok || m != 7*60+5 {
		t.Fatalf("7:05 → %d ok=%v", m, ok)
	}
	if FormatHHMM(m) != "07:05" {
		t.Fatal(FormatHHMM(m))
	}
	if _, ok := ParseHHMM("25:00"); ok {
		t.Fatal("25:00")
	}
}

func TestDayAllowed(t *testing.T) {
	if !DayAllowed(nil, time.Monday) {
		t.Fatal("empty = every day")
	}
	if !DayAllowed([]int{1, 2, 3, 4, 5}, time.Wednesday) {
		t.Fatal("weekday")
	}
	if DayAllowed([]int{1, 2, 3, 4, 5}, time.Sunday) {
		t.Fatal("Sunday not in weekdays")
	}
}

func TestSetDefaultTZ(t *testing.T) {
	prev := DefaultTZ()
	t.Cleanup(func() { SetDefaultTZ(prev) })
	SetDefaultTZ("Asia/Bangkok")
	if DefaultTZ() != "Asia/Bangkok" {
		t.Fatal(DefaultTZ())
	}
	loc, err := ResolveLocation("")
	if err != nil || loc.String() != "Asia/Bangkok" {
		t.Fatalf("empty tz → %v %v", loc, err)
	}
	SetDefaultTZ("Not/AZone")
	if DefaultTZ() != "UTC" {
		t.Fatal("invalid default → UTC")
	}
	if _, err := ResolveLocation("Nope/Nope"); err == nil {
		t.Fatal("expected unknown tz error")
	}
}
