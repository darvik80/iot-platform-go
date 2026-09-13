package plugins

import (
	"log/slog"
	"testing"
	"time"
)

func TestLEDFrameOffBlinkChase(t *testing.T) {
	strips := []ledStrip{{GPIO: 16, Length: 8}, {GPIO: 17, Length: 8}}

	off := LEDFrame(strips, "off", 0)
	if len(off) != 2 || off[0].On || off[0].Length != 8 || off[1].GPIO != 17 {
		t.Fatalf("off=%+v", off)
	}

	blinkOn := LEDFrame(strips, "blink", 0)
	if !blinkOn[0].On || blinkOn[0].Length != 8 {
		t.Fatalf("blink on=%+v", blinkOn)
	}
	blinkOff := LEDFrame(strips, "blink", 4)
	if blinkOff[0].On {
		t.Fatalf("blink off=%+v", blinkOff)
	}

	chase := LEDFrame([]ledStrip{{GPIO: 16, Length: 8}}, "chase", 3)
	if len(chase) != 2 || chase[0].On || chase[1].Offset != 3 || !chase[1].On || chase[1].Length != 1 {
		t.Fatalf("chase=%+v", chase)
	}

	if n := len(LEDFrame(strips, "comet", 2)); n < 4 {
		t.Fatalf("comet cmds=%d", n)
	}
	scan := LEDFrame([]ledStrip{{GPIO: 16, Length: 8}}, "scanner", 9)
	if len(scan) != 2 || scan[1].Offset != 5 {
		t.Fatalf("scanner=%+v", scan)
	}
	if cmds := LEDFrame(nil, "rainbow", 0); len(cmds) != 0 {
		t.Fatalf("empty strips=%+v", cmds)
	}
	if normalizeEffect("police") != "police" || normalizeEffect("nope") != "off" {
		t.Fatal("normalizeEffect")
	}
}

func TestGarlandTicksSelectedEffect(t *testing.T) {
	g := &LEDGarland{}
	if err := g.ApplyConfig([]byte(`{"mode":"effect","effect":"chase","interval_ms":150}`)); err != nil {
		t.Fatal(err)
	}
	if !g.Auto() || g.IntervalMS() != 150 {
		t.Fatalf("chase should tick every 150ms, Auto=%v interval=%d", g.Auto(), g.IntervalMS())
	}
	if err := g.ApplyConfig([]byte(`{"mode":"effect","effect":"off"}`)); err != nil {
		t.Fatal(err)
	}
	if g.Auto() {
		t.Fatal("off should not tick")
	}
	if err := g.ApplyConfig([]byte(`{"mode":"auto","effect":"off"}`)); err != nil {
		t.Fatal(err)
	}
	if !g.Auto() {
		t.Fatal("auto mode should tick")
	}
}

func TestQuietHoursAndNightLight(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	cfg := ledConfig{
		Mode: "effect", Effect: "rainbow", IntervalMS: 200, AutoMS: 8000,
		Quiet:  ledWindow{Enabled: true, From: "23:00", To: "07:00", Effect: "off"},
		Active: ledWindow{Enabled: true, From: "18:00", To: "23:30"},
	}
	night := time.Date(2026, 8, 9, 23, 15, 0, 0, loc)
	if got := resolveScheduleEffect(cfg, loc, night, 0); got != "off" {
		t.Fatalf("quiet+active overlap at 23:15 → %s", got)
	}
	evening := time.Date(2026, 8, 9, 20, 0, 0, 0, loc)
	if got := resolveScheduleEffect(cfg, loc, evening, 0); got != "rainbow" {
		t.Fatalf("evening → %s", got)
	}
	noon := time.Date(2026, 8, 9, 12, 0, 0, 0, loc)
	if got := resolveScheduleEffect(cfg, loc, noon, 0); got != "off" {
		t.Fatalf("outside active → %s", got)
	}
	warmNight := cfg
	warmNight.Quiet.Effect = "warm"
	late := time.Date(2026, 8, 10, 1, 0, 0, 0, loc)
	if got := resolveScheduleEffect(warmNight, loc, late, 0); got != "off" {
		t.Fatalf("01:00 is outside active window → %s", got)
	}
	onlyQuiet := ledConfig{
		Mode: "effect", Effect: "chase", IntervalMS: 200,
		Quiet: ledWindow{Enabled: true, From: "23:00", To: "07:00", Effect: "warm"},
	}
	if got := resolveScheduleEffect(onlyQuiet, loc, late, 0); got != "warm" {
		t.Fatalf("quiet warm → %s", got)
	}
	if got := resolveScheduleEffect(onlyQuiet, loc, noon, 0); got != "chase" {
		t.Fatalf("day chase → %s", got)
	}
}

func TestGarlandAutoWithQuietHours(t *testing.T) {
	g := &LEDGarland{}
	if err := g.ApplyConfig([]byte(`{"mode":"effect","effect":"off","quiet":{"enabled":true,"from":"23:00","to":"07:00","effect":"off"}}`)); err != nil {
		t.Fatal(err)
	}
	if !g.Auto() {
		t.Fatal("quiet hours should keep ticker so windows apply")
	}
}

type recRT struct {
	calls []rpcCall
}

type rpcCall struct {
	method string
	params map[string]any
}

func (r *recRT) InvokeRPC(method string, params map[string]any) error {
	r.calls = append(r.calls, rpcCall{method, params})
	return nil
}

func (r *recRT) Log() *slog.Logger {
	return slog.Default()
}

func TestGarlandLedAttachOnApplyAndOnline(t *testing.T) {
	g := &LEDGarland{}
	if err := g.ApplyConfig([]byte(`{"strips":[{"gpio":16,"length":8},{"gpio":17,"length":8}],"mode":"effect","effect":"off","brightness":40}`)); err != nil {
		t.Fatal(err)
	}
	rt := &recRT{}
	if err := g.OnEvent(rt, Event{Kind: EventApply}); err != nil {
		t.Fatal(err)
	}
	if len(rt.calls) < 4 {
		t.Fatalf("want 2×led_attach + 2×set_led, got %+v", rt.calls)
	}
	if rt.calls[0].method != "led_attach" || rt.calls[0].params["gpio"] != 16 || rt.calls[0].params["count"] != 8 || rt.calls[0].params["brightness"] != 40 {
		t.Fatalf("attach0=%+v", rt.calls[0])
	}
	if rt.calls[1].method != "led_attach" || rt.calls[1].params["gpio"] != 17 {
		t.Fatalf("attach1=%+v", rt.calls[1])
	}
	if rt.calls[2].method != "set_led" {
		t.Fatalf("paint=%+v", rt.calls[2])
	}

	tick := &recRT{}
	if err := g.OnEvent(tick, Event{Kind: EventTick}); err != nil {
		t.Fatal(err)
	}
	for _, c := range tick.calls {
		if c.method == "led_attach" {
			t.Fatal("tick must not re-attach while still bound")
		}
	}

	if err := g.OnEvent(&recRT{}, Event{Kind: EventStatus, Online: false}); err != nil {
		t.Fatal(err)
	}
	online := &recRT{}
	if err := g.OnEvent(online, Event{Kind: EventStatus, Online: true}); err != nil {
		t.Fatal(err)
	}
	if len(online.calls) < 1 || online.calls[0].method != "led_attach" {
		t.Fatalf("online should attach again, got %+v", online.calls)
	}
}

func TestLEDAttachParams(t *testing.T) {
	got := ledAttachParams([]ledStrip{{GPIO: 16, Length: 8}, {GPIO: 17, Length: 0}}, 0)
	if len(got) != 1 || got[0]["gpio"] != 16 || got[0]["count"] != 8 || got[0]["brightness"] != 32 {
		t.Fatalf("%+v", got)
	}
}

func TestHSVRed(t *testing.T) {
	r, g, b := hsvRGB(0, 1, 1)
	if r != 255 || g != 0 || b != 0 {
		t.Fatalf("hsv red=%d,%d,%d", r, g, b)
	}
}
