package plugins

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	defaultTZMu sync.RWMutex
	defaultTZ   = "UTC"
)

// SetDefaultTZ is the platform TIMEZONE (IANA). Empty plugin tz falls back here.
func SetDefaultTZ(tz string) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		tz = "UTC"
	}
	defaultTZMu.Lock()
	defaultTZ = tz
	defaultTZMu.Unlock()
}

func DefaultTZ() string {
	defaultTZMu.RLock()
	defer defaultTZMu.RUnlock()
	return defaultTZ
}

func ResolveLocation(tz string) (*time.Location, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		tz = DefaultTZ()
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone %q", tz)
	}
	return loc, nil
}

// ParseHHMM returns minutes from midnight (0–1439).
func ParseHHMM(s string) (int, bool) {
	s = strings.TrimSpace(s)
	var h, m int
	n, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	if err == nil && n == 2 && h >= 0 && h < 24 && m >= 0 && m < 60 {
		return h*60 + m, true
	}
	if _, err := fmt.Sscanf(s, "%d", &h); err == nil && h >= 0 && h < 24 {
		return h * 60, true
	}
	return 0, false
}

func FormatHHMM(minutes int) string {
	if minutes < 0 {
		minutes = 0
	}
	minutes %= 24 * 60
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// InWindow is true if now (minutes from midnight) is inside [from, to).
// If from > to the window wraps midnight (e.g. 22:00–07:00).
// from == to means the whole day.
func InWindow(now, from, to int) bool {
	const day = 24 * 60
	now = ((now % day) + day) % day
	from = ((from % day) + day) % day
	to = ((to % day) + day) % day
	if from == to {
		return true
	}
	if from < to {
		return now >= from && now < to
	}
	return now >= from || now < to
}

func DayAllowed(days []int, wd time.Weekday) bool {
	if len(days) == 0 {
		return true
	}
	w := int(wd)
	for _, d := range days {
		if d == w {
			return true
		}
	}
	return false
}
