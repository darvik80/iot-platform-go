package plugins

import (
	"testing"
	"time"
)

func TestJobsDueAtOncePerMinute(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	jobs := []schedJob{
		{Name: "off", At: "23:00", RPC: "set_led", Params: map[string]any{"on": false}},
		{Name: "reboot", At: "04:00", Days: []int{1, 2, 3, 4, 5}, RPC: "reboot"},
	}
	fired := map[string]string{}
	sunNight := time.Date(2026, 8, 9, 23, 0, 0, 0, loc) // Sunday
	due := jobsDueAt(jobs, sunNight, fired)
	if len(due) != 1 || due[0].Name != "off" {
		t.Fatalf("sunday 23:00 due=%+v", due)
	}
	due = jobsDueAt(jobs, sunNight, fired)
	if len(due) != 0 {
		t.Fatalf("duplicate fire %+v", due)
	}
	monReboot := time.Date(2026, 8, 10, 4, 0, 0, 0, loc) // Monday
	due = jobsDueAt(jobs, monReboot, fired)
	if len(due) != 1 || due[0].Name != "reboot" {
		t.Fatalf("monday 04:00 due=%+v", due)
	}
	sunReboot := time.Date(2026, 8, 9, 4, 0, 0, 0, loc)
	due = jobsDueAt(jobs, sunReboot, fired)
	if len(due) != 0 {
		t.Fatalf("sunday reboot should skip %+v", due)
	}
}

func TestScheduleApplyConfig(t *testing.T) {
	s := &Schedule{}
	if err := s.ApplyConfig([]byte(`{"tz":"Asia/Bangkok","jobs":[{"at":"7:30","rpc":"reboot"}]}`)); err != nil {
		t.Fatal(err)
	}
	if !s.Auto() || s.IntervalMS() != 10000 {
		t.Fatal("schedule always ticks")
	}
	if err := s.ApplyConfig([]byte(`{"tz":"Nope"}`)); err == nil {
		t.Fatal("expected bad tz")
	}
}
