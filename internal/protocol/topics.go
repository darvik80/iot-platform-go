// Package protocol is the MQTT v1 topic contract (full + short mnemonics).
// Device ingest, downlinks, ACL, and the rules bus all use these names.
package protocol

import (
	"fmt"
	"strings"
)

const (
	StyleFull  = "full"
	StyleShort = "short"
)

const (
	PrefixFull  = "iot/v1/"
	PrefixShort = "v1/"
	DirUp       = "up"
	DirDown     = "down"
	Me          = "me"
	SegEvents   = "events"
	SegCommands = "commands"
	SegAccount  = "account"
)

// Canonical ingest kinds (full tail and short mnemonic fold here).
const (
	KindStatus      = "status"
	KindTelemetry   = "telemetry"
	KindEvent       = "event"
	KindAttrReport  = "attributes/report"
	KindAttrRequest = "attributes/request"
	KindRPCResponse = "rpc/response"
	KindRPCRequest  = "rpc/request"
	KindNTPRequest  = "ntp/request"
	KindOTAVersion  = "ota/version"
	KindOTAQuery    = "ota/query"
	KindOTAProgress = "ota/progress"
	KindLogs        = "logs"
)

// Full uplink tails after iot/v1/{p}/{d}/up/.
const (
	UpStatus      = "status"
	UpTelemetry   = "telemetry/data"
	UpEvent       = "events/post"
	UpAttrReport  = "attributes/report"
	UpAttrRequest = "attributes/request"
	UpRPCResponse = "rpc/response"
	UpRPCRequest  = "rpc/request"
	UpNTPRequest  = "ntp/request"
	UpOTAVersion  = "ota/version"
	UpOTAQuery    = "ota/query"
	UpOTAProgress = "ota/progress"
	UpLogs        = "logs/report"
)

// Short uplink tails after v1/ or v1/me/.
const (
	ShortStatus      = "s"
	ShortTelemetry   = "t"
	ShortEvent       = "e"
	ShortAttr        = "a"
	ShortAttrRequest = "a/req"
	ShortRPCResponse = "r/res"
	ShortRPCClient   = "r/creq"
	ShortNTPRequest  = "n/req"
	ShortOTAVersion  = "o/ver"
	ShortOTAQuery    = "o/q"
	ShortOTAProgress = "o/p"
	ShortLogs        = "l"
)

// Downlink capability / operation (Platform.DownTopic).
const (
	CapAttributes = "attributes"
	CapRPC        = "rpc"
	CapNTP        = "ntp"
	CapOTA        = "ota"

	OpResponse = "response"
	OpUpdate   = "update"
	OpRequest  = "request"
	OpCancel   = "cancel"
)

const (
	ShortAttrResponse = "a/res"
	ShortAttrUpdate   = "a/upd"
	ShortRPCRequest   = "r/req"
	ShortRPCClientRes = "r/cres"
	ShortNTPResponse  = "n/res"
	ShortOTAUpdate    = "o/upd"
	ShortOTACancel    = "o/can"
)

// Rules bus event / command kinds under {RULES_BUS_PREFIX}/events|commands/…
const (
	BusStatus             = "status"
	BusTelemetry          = "telemetry"
	BusAttributesReported = "attributes/reported"
	BusAttributesDesired  = "attributes/desired"
	BusEvent              = "event"
	BusRPCResult          = "rpc/result"
	BusOTA                = "ota"
	BusLog                = "log"

	CmdRPC               = "rpc"
	CmdAttributesDesired = "attributes/desired"
	CmdOTA               = "ota"
)

var mePrefixes = []string{"devices/me/", "dev/me/", "me/"}

func ValidStyle(s string) bool {
	return s == StyleFull || s == StyleShort
}

func DetectStyle(topic string) string {
	t := strings.TrimSpace(topic)
	if t == "" || strings.HasPrefix(t, "$") {
		return ""
	}
	if strings.HasPrefix(t, PrefixFull) {
		return StyleFull
	}
	if strings.HasPrefix(t, PrefixShort) {
		return StyleShort
	}
	return ""
}

func UnwrapMe(capOp string) string {
	for _, p := range mePrefixes {
		if rest, ok := strings.CutPrefix(capOp, p); ok {
			return rest
		}
	}
	return capOp
}

// NormalizeCapOp maps a topic tail (full or short, optional /me/) to a Kind*.
func NormalizeCapOp(capOp string) string {
	capOp = UnwrapMe(capOp)
	switch {
	case capOp == UpStatus || capOp == ShortStatus:
		return KindStatus
	case capOp == UpTelemetry || capOp == ShortTelemetry || capOp == "telemetry":
		return KindTelemetry
	case capOp == UpEvent || capOp == ShortEvent:
		return KindEvent
	case capOp == UpAttrReport || capOp == ShortAttr || capOp == "attributes":
		return KindAttrReport
	case capOp == UpAttrRequest || capOp == ShortAttrRequest || strings.HasPrefix(capOp, UpAttrRequest):
		return KindAttrRequest
	case capOp == UpRPCResponse || capOp == ShortRPCResponse || strings.HasPrefix(capOp, UpRPCResponse):
		return KindRPCResponse
	case capOp == UpRPCRequest || capOp == ShortRPCClient || strings.HasPrefix(capOp, UpRPCRequest):
		return KindRPCRequest
	case capOp == UpNTPRequest || capOp == ShortNTPRequest:
		return KindNTPRequest
	case capOp == UpOTAVersion || capOp == ShortOTAVersion:
		return KindOTAVersion
	case capOp == UpOTAQuery || capOp == ShortOTAQuery:
		return KindOTAQuery
	case capOp == UpOTAProgress || capOp == ShortOTAProgress:
		return KindOTAProgress
	case capOp == UpLogs || capOp == ShortLogs:
		return KindLogs
	default:
		return capOp
	}
}

func FullUpRoot(productID, deviceID string) string {
	return PrefixFull + productID + "/" + deviceID + "/" + DirUp + "/"
}

func FullDownRoot(productID, deviceID string) string {
	return PrefixFull + productID + "/" + deviceID + "/" + DirDown + "/"
}

func ShortMe(tail string) string {
	if tail == "" {
		return PrefixShort + Me
	}
	return PrefixShort + Me + "/" + tail
}

func RulesEventsRoot(prefix string) string {
	return strings.TrimRight(prefix, "/") + "/" + SegEvents + "/"
}

func RulesCommandsRoot(prefix string) string {
	return strings.TrimRight(prefix, "/") + "/" + SegCommands + "/"
}

// RulesEventsRootForAccount returns the account-scoped events root.
// Format: {prefix}/events/{accountSlug}/
func RulesEventsRootForAccount(prefix, accountSlug string) string {
	return strings.TrimRight(prefix, "/") + "/" + SegEvents + "/" + accountSlug + "/"
}

// RulesCommandsRootForAccount returns the account-scoped commands root.
// Format: {prefix}/commands/{accountSlug}/
func RulesCommandsRootForAccount(prefix, accountSlug string) string {
	return strings.TrimRight(prefix, "/") + "/" + SegCommands + "/" + accountSlug + "/"
}

func DownTopic(style, productID, deviceID, capability, operation string) string {
	if style == StyleFull {
		if operation == "" {
			return fmt.Sprintf("%s%s/%s/%s/%s", PrefixFull, productID, deviceID, DirDown, capability)
		}
		return fmt.Sprintf("%s%s/%s/%s/%s/%s", PrefixFull, productID, deviceID, DirDown, capability, operation)
	}
	switch capability + "/" + operation {
	case CapAttributes + "/" + OpResponse:
		return ShortMe(ShortAttrResponse)
	case CapAttributes + "/" + OpUpdate:
		return ShortMe(ShortAttrUpdate)
	case CapRPC + "/" + OpRequest:
		return ShortMe(ShortRPCRequest)
	case CapRPC + "/" + OpResponse:
		return ShortMe(ShortRPCClientRes)
	case CapNTP + "/" + OpResponse:
		return ShortMe(ShortNTPResponse)
	case CapOTA + "/" + OpUpdate:
		return ShortMe(ShortOTAUpdate)
	case CapOTA + "/" + OpCancel:
		return ShortMe(ShortOTACancel)
	default:
		return ShortMe(capability)
	}
}
