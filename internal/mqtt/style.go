package mqtt

import "strings"

// DetectTopicStyle infers short vs full from a publish topic or subscribe filter.
//
//	iot/v1/{product}/{device}/…  → full (identity in path)
//	v1/me/…, v1/dev/me/…        → short (session prefix, TB-like /me/)
//	v1/t, v1/r/req, …           → short (mnemonics)
func DetectTopicStyle(topic string) string {
	t := strings.TrimSpace(topic)
	if t == "" {
		return ""
	}
	if strings.HasPrefix(t, "$") {
		return ""
	}
	if strings.HasPrefix(t, "iot/v1/") {
		return "full"
	}
	if strings.HasPrefix(t, "v1/") {
		return "short"
	}
	return ""
}

// shortCapOp maps session-scoped topic tails to ingest capability/operation.
// v1/me/… (and aliases v1/dev/me, v1/devices/me) unwrap to the same mnemonics as v1/t.
func shortCapOp(capOp string) string {
	for _, p := range []string{"dev/me/", "me/"} {
		if rest, ok := strings.CutPrefix(capOp, p); ok {
			capOp = rest
			break
		}
	}
	switch {
	case capOp == "telemetry":
		return "t"
	case capOp == "attributes":
		return "a"
	case strings.HasPrefix(capOp, "attributes/request"):
		return "a/req"
	case strings.HasPrefix(capOp, "rpc/response"):
		return "r/res"
	case strings.HasPrefix(capOp, "rpc/request"):
		return "r/creq"
	default:
		return capOp
	}
}
