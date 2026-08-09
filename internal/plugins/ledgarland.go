package plugins

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"
)

const PluginLEDGarland = "led-garland"

var ledGarlandDefault = json.RawMessage(`{"strips":[{"gpio":16,"length":8},{"gpio":17,"length":8}],"mode":"effect","effect":"off","interval_ms":200,"auto_ms":8000,"rpc":"set_led","attach_rpc":"led_attach","brightness":32,"tz":"","quiet":{"enabled":false,"from":"23:00","to":"07:00","effect":"off"},"active":{"enabled":false,"from":"18:00","to":"23:00"}}`)

func init() {
	Register(Descriptor{
		ID:            PluginLEDGarland,
		Name:          "LED garland",
		Description:   "WS2812 strips: led_attach on save/online, then set_led effects",
		DefaultConfig: ledGarlandDefault,
		Effects:       LEDEffects,
	}, func() Instance { return &LEDGarland{} })
}

// LEDEffects is the full catalog shown in the UI.
var LEDEffects = []string{
	"off", "white", "warm", "red", "green", "blue",
	"blink", "strobe", "breathe",
	"chase", "comet", "scanner", "fill", "theater", "sparkle",
	"fire", "rainbow", "police", "christmas",
}

// LEDAutoEffects are cycled in mode=auto (skip static solids / off).
var LEDAutoEffects = []string{
	"blink", "strobe", "breathe", "chase", "comet", "scanner",
	"fill", "theater", "sparkle", "fire", "rainbow", "police", "christmas",
}

type ledStrip struct {
	GPIO   int `json:"gpio"`
	Length int `json:"length"`
}

type ledWindow struct {
	Enabled bool   `json:"enabled"`
	From    string `json:"from"`
	To      string `json:"to"`
	Effect  string `json:"effect,omitempty"`
}

type ledConfig struct {
	Strips     []ledStrip `json:"strips"`
	Mode       string     `json:"mode"` // auto | effect
	Effect     string     `json:"effect"`
	IntervalMS int        `json:"interval_ms"`
	AutoMS     int        `json:"auto_ms"`
	RPC        string     `json:"rpc"`
	AttachRPC  string     `json:"attach_rpc"`
	Brightness int        `json:"brightness"`
	TZ         string     `json:"tz"`
	Quiet      ledWindow  `json:"quiet"`
	Active     ledWindow  `json:"active"`
}

type LEDGarland struct {
	mu       sync.Mutex
	cfg      ledConfig
	loc      *time.Location
	frame    int
	lastSent string
	attached bool
}

func (g *LEDGarland) ApplyConfig(raw json.RawMessage) error {
	cfg := ledConfig{
		Mode:       "effect",
		Effect:     "off",
		IntervalMS: 200,
		AutoMS:     8000,
		RPC:        "set_led",
		AttachRPC:  "led_attach",
		Brightness: 32,
		Strips:     []ledStrip{{GPIO: 16, Length: 8}, {GPIO: 17, Length: 8}},
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	if cfg.RPC == "" {
		cfg.RPC = "set_led"
	}
	if cfg.AttachRPC == "" {
		cfg.AttachRPC = "led_attach"
	}
	if cfg.Brightness < 1 {
		cfg.Brightness = 32
	}
	if cfg.Brightness > 255 {
		cfg.Brightness = 255
	}
	if cfg.IntervalMS < 80 {
		cfg.IntervalMS = 80
	}
	if cfg.IntervalMS > 5000 {
		cfg.IntervalMS = 5000
	}
	if cfg.AutoMS < 1000 {
		cfg.AutoMS = 8000
	}
	if cfg.Mode != "auto" {
		cfg.Mode = "effect"
	}
	cfg.Effect = normalizeEffect(cfg.Effect)
	if cfg.Strips == nil {
		cfg.Strips = []ledStrip{}
	}
	for i := range cfg.Strips {
		if cfg.Strips[i].Length < 1 {
			cfg.Strips[i].Length = 1
		}
		if cfg.Strips[i].Length > 256 {
			cfg.Strips[i].Length = 256
		}
	}
	loc, err := ResolveLocation(cfg.TZ)
	if err != nil {
		return err
	}
	cfg.Quiet = normalizeWindow(cfg.Quiet, "off")
	cfg.Active = normalizeWindow(cfg.Active, "")
	g.mu.Lock()
	g.cfg = cfg
	g.loc = loc
	g.frame = 0
	g.lastSent = ""
	g.attached = false
	g.mu.Unlock()
	return nil
}

func normalizeWindow(w ledWindow, defaultEffect string) ledWindow {
	if w.From == "" {
		w.From = "23:00"
	}
	if w.To == "" {
		w.To = "07:00"
	}
	if from, ok := ParseHHMM(w.From); ok {
		w.From = FormatHHMM(from)
	} else {
		w.Enabled = false
	}
	if to, ok := ParseHHMM(w.To); ok {
		w.To = FormatHHMM(to)
	} else {
		w.Enabled = false
	}
	if defaultEffect != "" {
		if w.Effect == "" {
			w.Effect = defaultEffect
		}
		w.Effect = normalizeEffect(w.Effect)
	}
	return w
}

func (g *LEDGarland) IntervalMS() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	effect := resolveScheduleEffect(g.cfg, g.loc, time.Now(), g.frame)
	if effectNeedsTick(effect) {
		return g.cfg.IntervalMS
	}
	if g.hasScheduleLocked() {
		return 15000
	}
	return g.cfg.IntervalMS
}

func (g *LEDGarland) Auto() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.hasScheduleLocked() || g.cfg.Mode == "auto" {
		return true
	}
	return effectNeedsTick(g.cfg.Effect)
}

func (g *LEDGarland) hasScheduleLocked() bool {
	return g.cfg.Quiet.Enabled || g.cfg.Active.Enabled
}

func resolveScheduleEffect(cfg ledConfig, loc *time.Location, now time.Time, frame int) string {
	if loc != nil {
		now = now.In(loc)
	}
	mins := now.Hour()*60 + now.Minute()
	if cfg.Active.Enabled {
		from, ok1 := ParseHHMM(cfg.Active.From)
		to, ok2 := ParseHHMM(cfg.Active.To)
		if ok1 && ok2 && !InWindow(mins, from, to) {
			return "off"
		}
	}
	if cfg.Quiet.Enabled {
		from, ok1 := ParseHHMM(cfg.Quiet.From)
		to, ok2 := ParseHHMM(cfg.Quiet.To)
		if ok1 && ok2 && InWindow(mins, from, to) {
			if cfg.Quiet.Effect == "" {
				return "off"
			}
			return normalizeEffect(cfg.Quiet.Effect)
		}
	}
	if cfg.Mode == "auto" {
		cycle := cfg.AutoMS / cfg.IntervalMS
		if cycle < 1 {
			cycle = 1
		}
		return LEDAutoEffects[(frame/cycle)%len(LEDAutoEffects)]
	}
	return cfg.Effect
}

func effectNeedsTick(effect string) bool {
	switch normalizeEffect(effect) {
	case "off", "white", "warm", "red", "green", "blue":
		return false
	default:
		return true
	}
}

func (g *LEDGarland) OnEvent(rt Runtime, ev Event) error {
	switch ev.Kind {
	case EventStatus:
		if !ev.Online {
			g.mu.Lock()
			g.attached = false
			g.lastSent = ""
			g.mu.Unlock()
			return nil
		}
		if err := g.attach(rt, true); err != nil {
			return err
		}
		return g.paint(rt, false)
	case EventApply:
		if err := g.attach(rt, true); err != nil {
			return err
		}
		return g.paint(rt, false)
	case EventTick:
		if err := g.attach(rt, false); err != nil {
			return err
		}
		return g.paint(rt, true)
	default:
		return nil
	}
}

func ledAttachParams(strips []ledStrip, brightness int) []map[string]any {
	if brightness < 1 {
		brightness = 32
	}
	if brightness > 255 {
		brightness = 255
	}
	var out []map[string]any
	for _, s := range strips {
		if s.Length < 1 {
			continue
		}
		out = append(out, map[string]any{
			"gpio": s.GPIO, "count": s.Length, "brightness": brightness,
		})
	}
	return out
}

func (g *LEDGarland) attach(rt Runtime, force bool) error {
	g.mu.Lock()
	if !force && g.attached {
		g.mu.Unlock()
		return nil
	}
	strips := append([]ledStrip(nil), g.cfg.Strips...)
	method := g.cfg.AttachRPC
	bri := g.cfg.Brightness
	g.mu.Unlock()
	if method == "" {
		method = "led_attach"
	}
	for _, p := range ledAttachParams(strips, bri) {
		if err := rt.InvokeRPC(method, p); err != nil {
			return err
		}
	}
	g.mu.Lock()
	g.attached = true
	g.mu.Unlock()
	return nil
}

func (g *LEDGarland) paint(rt Runtime, skipDupStatic bool) error {
	g.mu.Lock()
	cfg := g.cfg
	loc := g.loc
	frame := g.frame
	g.frame++
	effect := resolveScheduleEffect(cfg, loc, time.Now(), frame)
	skip := skipDupStatic && !effectNeedsTick(effect) && g.lastSent == effect
	if !skip {
		g.lastSent = effect
	}
	g.mu.Unlock()
	if skip {
		return nil
	}
	for _, cmd := range LEDFrame(cfg.Strips, effect, frame) {
		params := map[string]any{
			"gpio": cmd.GPIO, "offset": cmd.Offset, "length": cmd.Length,
			"r": cmd.R, "g": cmd.G, "b": cmd.B, "on": cmd.On,
		}
		if err := rt.InvokeRPC(cfg.RPC, params); err != nil {
			return err
		}
	}
	return nil
}

type LEDCommand struct {
	GPIO    int
	Offset  int
	Length  int
	R, G, B int
	On      bool
}

func LEDFrame(strips []ledStrip, effect string, frame int) []LEDCommand {
	effect = normalizeEffect(effect)
	var out []LEDCommand
	for _, s := range strips {
		n := s.Length
		if n < 1 {
			continue
		}
		switch effect {
		case "off":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
		case "white":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: 255, G: 255, B: 255, On: true})
		case "warm":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: 255, G: 170, B: 60, On: true})
		case "red":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: 255, On: true})
		case "green":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, G: 220, On: true})
		case "blue":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, B: 255, On: true})
		case "blink":
			on := (frame/4)%2 == 0
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: 255, G: 80, B: 20, On: on})
		case "strobe":
			on := frame%2 == 0
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: 255, G: 255, B: 255, On: on})
		case "breathe":
			v := (math.Sin(float64(frame)*0.18) + 1) / 2
			r, g, b := hsvRGB(30, 0.9, 0.15+0.85*v)
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: r, G: g, B: b, On: true})
		case "chase":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: frame % n, Length: 1, R: 0, G: 220, B: 255, On: true})
		case "comet":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
			head := frame % n
			for k, bri := range []float64{1, 0.45, 0.18} {
				off := (head - k + n) % n
				r, g, b := hsvRGB(200, 0.85, bri)
				out = append(out, LEDCommand{GPIO: s.GPIO, Offset: off, Length: 1, R: r, G: g, B: b, On: true})
			}
		case "scanner":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
			cycle := n*2 - 2
			if cycle < 1 {
				cycle = 1
			}
			p := frame % cycle
			if p >= n {
				p = cycle - p
			}
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: p, Length: 1, R: 255, G: 30, B: 30, On: true})
		case "fill":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
			l := frame % (n + 1)
			if l > 0 {
				r, g, b := hsvRGB(float64((frame*3)%360), 0.9, 1)
				out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: l, R: r, G: g, B: b, On: true})
			}
		case "theater":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
			for i := frame % 3; i < n; i += 3 {
				out = append(out, LEDCommand{GPIO: s.GPIO, Offset: i, Length: 1, R: 255, G: 200, B: 40, On: true})
			}
		case "sparkle":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
			a := (frame * 3) % n
			b := (frame*7 + 4) % n
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: a, Length: 1, R: 255, G: 255, B: 255, On: true})
			if b != a {
				out = append(out, LEDCommand{GPIO: s.GPIO, Offset: b, Length: 1, R: 180, G: 220, B: 255, On: true})
			}
		case "fire":
			v := 0.45 + 0.55*math.Abs(math.Sin(float64(frame)*0.7+float64(s.GPIO)))
			r, g, b := hsvRGB(18+float64(frame%8), 1, v)
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: r, G: g, B: b, On: true})
		case "rainbow":
			r, g, b := hsvRGB(float64((frame*8)%360), 1, 1)
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: r, G: g, B: b, On: true})
		case "police":
			if frame%2 == 0 {
				out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, R: 255, On: true})
			} else {
				out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, B: 255, On: true})
			}
		case "christmas":
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
			for i := 0; i < n; i++ {
				if (i+frame)%2 == 0 {
					out = append(out, LEDCommand{GPIO: s.GPIO, Offset: i, Length: 1, R: 220, G: 0, B: 0, On: true})
				} else {
					out = append(out, LEDCommand{GPIO: s.GPIO, Offset: i, Length: 1, R: 0, G: 180, B: 40, On: true})
				}
			}
		default:
			out = append(out, LEDCommand{GPIO: s.GPIO, Offset: 0, Length: n, On: false})
		}
	}
	return out
}

func normalizeEffect(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, e := range LEDEffects {
		if e == s {
			return s
		}
	}
	return "off"
}

func hsvRGB(h, s, v float64) (int, int, int) {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return clamp255((r + m) * 255), clamp255((g + m) * 255), clamp255((b + m) * 255)
}

func clamp255(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(math.Round(v))
}
