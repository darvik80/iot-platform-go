package plugins

import "encoding/json"

var ledGarlandForm = []FormField{
	{
		Key: "strips", Type: "array", Label: "Strips",
		AddLabel: "Add strip", RemoveLabel: "Delete strip",
		Add: json.RawMessage(`{"gpio":16,"length":8}`),
		Fields: []FormField{
			{Key: "gpio", Type: "number", Label: "GPIO", Min: num(0), Suggest: []int{16, 17, 18, 19, 21, 22, 23, 25, 26, 27, 32, 33}},
			{Key: "length", Type: "number", Label: "LEDs", Min: num(1)},
		},
	},
	{Key: "mode", Type: "select", Label: "Mode", Options: []FormOpt{{"effect", "effect"}, {"auto", "auto (cycle)"}}},
	{Key: "effect", Type: "select", Label: "Effect", OptionsFrom: "effects", Options: ledEffectOpts, DisabledWhen: map[string]any{"mode": "auto"}},
	{Key: "interval_ms", Type: "number", Label: "Frame interval ms", Min: num(80)},
	{Key: "brightness", Type: "number", Label: "Brightness", Min: num(1), Max: num(255)},
	{Key: "tz", Type: "timezone", Label: "Timezone"},
	{
		Key: "quiet", Type: "object", Label: "Quiet hours (wraps midnight)",
		Fields: []FormField{
			{Key: "enabled", Type: "checkbox", Label: "Enabled"},
			{Key: "from", Type: "time", Label: "From"},
			{Key: "to", Type: "time", Label: "To"},
			{Key: "effect", Type: "select", Label: "Effect", Options: []FormOpt{
				{"off", "Off"}, {"warm", "Warm white"}, {"white", "Solid white"}, {"breathe", "Breathe"},
			}},
		},
	},
	{
		Key: "active", Type: "object", Label: "Only between (night light)",
		Fields: []FormField{
			{Key: "enabled", Type: "checkbox", Label: "Enabled"},
			{Key: "from", Type: "time", Label: "From"},
			{Key: "to", Type: "time", Label: "To"},
		},
	},
}

var ledEffectOpts = []FormOpt{
	{"off", "Off"},
	{"white", "Solid white"},
	{"warm", "Warm white"},
	{"red", "Solid red"},
	{"green", "Solid green"},
	{"blue", "Solid blue"},
	{"blink", "Blink"},
	{"strobe", "Strobe"},
	{"breathe", "Breathe"},
	{"chase", "Chase"},
	{"comet", "Comet"},
	{"scanner", "Scanner (cylon)"},
	{"fill", "Fill"},
	{"theater", "Theater chase"},
	{"sparkle", "Sparkle"},
	{"fire", "Fire"},
	{"rainbow", "Rainbow wash"},
	{"police", "Police"},
	{"christmas", "Christmas"},
}
