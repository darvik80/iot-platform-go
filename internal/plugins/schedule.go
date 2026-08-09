package plugins

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const PluginSchedule = "schedule"

func init() {
	Register(Descriptor{
		ID:          PluginSchedule,
		Name:        "Schedule",
		Description: "Run device RPC at local times (uses platform TIMEZONE unless tz is set)",
		DefaultConfig: json.RawMessage(`{
			"tz":"",
			"jobs":[
				{"name":"night off","at":"23:00","days":[],"rpc":"set_led","params":{"gpio":16,"offset":0,"length":8,"on":false}}
			]
		}`),
		Form: scheduleForm,
	}, func() Instance { return &Schedule{} })
}

var scheduleForm = []FormField{
	{Key: "tz", Type: "timezone", Label: "Timezone"},
	{Key: "_jobs", Type: "break"},
	{
		Key: "jobs", Type: "array", Label: "Jobs",
		AddLabel: "Add job", RemoveLabel: "Delete job",
		Add: json.RawMessage(`{"name":"","at":"07:00","days":[],"rpc":"reboot","params":{}}`),
		Fields: []FormField{
			{Key: "name", Type: "string", Label: "Name"},
			{Key: "at", Type: "time", Label: "At"},
			{Key: "rpc", Type: "string", Label: "RPC"},
			{Key: "days", Type: "weekdays", Label: "Days", Help: "empty / all = every day", Wide: true},
			{Key: "params", Type: "json", Label: "Params JSON", Wide: true},
		},
	},
}

type schedJob struct {
	Name   string         `json:"name"`
	At     string         `json:"at"`
	Days   []int          `json:"days"`
	RPC    string         `json:"rpc"`
	Params map[string]any `json:"params"`
}

type schedConfig struct {
	TZ   string     `json:"tz"`
	Jobs []schedJob `json:"jobs"`
}

type Schedule struct {
	mu    sync.Mutex
	cfg   schedConfig
	loc   *time.Location
	fired map[string]string
}

func (s *Schedule) ApplyConfig(raw json.RawMessage) error {
	cfg := schedConfig{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	loc, err := ResolveLocation(cfg.TZ)
	if err != nil {
		return err
	}
	out := make([]schedJob, 0, len(cfg.Jobs))
	for _, j := range cfg.Jobs {
		j.RPC = strings.TrimSpace(j.RPC)
		if j.RPC == "" {
			continue
		}
		if _, ok := ParseHHMM(j.At); !ok {
			continue
		}
		j.At = FormatHHMM(mustMinutes(j.At))
		if j.Params == nil {
			j.Params = map[string]any{}
		}
		out = append(out, j)
	}
	cfg.Jobs = out
	s.mu.Lock()
	s.cfg = cfg
	s.loc = loc
	s.fired = map[string]string{}
	s.mu.Unlock()
	return nil
}

func (s *Schedule) IntervalMS() int { return 10000 }
func (s *Schedule) Auto() bool      { return true }

func (s *Schedule) OnEvent(rt Runtime, ev Event) error {
	if ev.Kind != EventTick && ev.Kind != EventApply {
		return nil
	}
	s.mu.Lock()
	cfg := s.cfg
	loc := s.loc
	if s.fired == nil {
		s.fired = map[string]string{}
	}
	var due []schedJob
	if loc != nil {
		now := time.Now().In(loc)
		due = jobsDueAt(cfg.Jobs, now, s.fired)
		today := now.Format("2006-01-02")
		for k := range s.fired {
			if !strings.HasPrefix(k, today) {
				delete(s.fired, k)
			}
		}
	}
	s.mu.Unlock()
	for _, j := range due {
		if err := rt.InvokeRPC(j.RPC, j.Params); err != nil {
			return err
		}
	}
	return nil
}

func jobsDueAt(jobs []schedJob, now time.Time, fired map[string]string) []schedJob {
	date := now.Format("2006-01-02")
	hm := FormatHHMM(now.Hour()*60 + now.Minute())
	var due []schedJob
	for _, j := range jobs {
		if !DayAllowed(j.Days, now.Weekday()) || j.At != hm {
			continue
		}
		key := date + ":" + j.At + ":" + j.RPC + ":" + j.Name
		if fired[key] != "" {
			continue
		}
		fired[key] = hm
		due = append(due, j)
	}
	return due
}

func mustMinutes(s string) int {
	m, _ := ParseHHMM(s)
	return m
}
