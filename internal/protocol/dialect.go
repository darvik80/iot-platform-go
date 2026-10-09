package protocol

import (
	"crypto/subtle"
	"fmt"
	"strings"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/models"
)

// Uplink is a parsed uplink topic. ProductID/DeviceID carry the identity the
// topic itself encodes; they are empty for session-scoped topics.
type Uplink struct {
	ProductID string
	DeviceID  string
	Kind      string // internal kind, already normalized
	Aux       string // event id or service method, when present
}

// Dialect is the wire contract of one device style (full, short, alink, …):
// auth, ACL roots, topic parsing, payload translation, downlinks, and
// credential presentation. Call sites branch on the dialect, never on the
// style string.
type Dialect interface {
	// Name is the devices.topic_style value.
	Name() string

	// Usernames lists broker usernames that resolve to the device.
	Usernames(d *models.Device) []string

	// VerifyPassword checks a CONNECT password; reason explains a failure.
	VerifyPassword(d *models.Device, password, clientID string) (ok bool, reason string)

	// CredsUsername is the username shown in device credentials; dialects
	// that do not derive it from product/device may use credential.
	CredsUsername(productID, deviceID, credential string) string

	// UsesSecret marks dialects whose credential is a raw secret stored on
	// the device row (HMAC key) rather than a hashed access token.
	UsesSecret() bool

	// Pinned marks dialects that must never be auto-detected away from: the
	// credential model changes with the style, so a foreign Will or filter
	// must not flip the device into a style it cannot authenticate in.
	Pinned() bool

	// Detects reports whether a topic or filter smells like the dialect.
	Detects(topicOrFilter string) bool

	// PublishRoots are the topic roots an uplink may appear under;
	// SubscribeRoots the roots a downlink may arrive on.
	PublishRoots(d *models.Device) []string
	SubscribeRoots(d *models.Device) []string

	// ParseUplink parses an uplink topic; ok is false for foreign shapes.
	ParseUplink(topic string) (Uplink, bool)

	// Uplink translates a parsed uplink payload into the internal shape.
	Uplink(kind, aux string, payload []byte) ([]byte, error)

	// Downlink builds the wire topic and body for a downlink; id seeds
	// message ids when the internal payload carries none.
	Downlink(d *models.Device, capability, operation, id string, payload []byte) (string, []byte, error)

	// DownTarget is the username a downlink must be delivered to; ""
	// means broadcast by topic.
	DownTarget(d *models.Device) string

	// DownViaDevice reports whether downlinks must be delivered directly to
	// the sessions of this device. True for dialects whose downlink topics
	// are shared across devices and whose sessions are only addressable
	// through the credential maps.
	DownViaDevice(d *models.Device) bool

	Snippet(productID, deviceID, host, credential string) string
	ConnectNote() string
}

// dialects is the parse/detect priority order. Topic prefixes are disjoint
// today; a dialect whose topics nest inside another's must come first.
var dialects = []Dialect{fullDialect{}, thingsboardDialect{}, shortDialect{}, alinkDialect{}}

var dialectByName = map[string]Dialect{}

func init() {
	for _, d := range dialects {
		dialectByName[d.Name()] = d
	}
}

// DialectFor returns the dialect registered for style. It falls back to the
// short dialect so a stale topic_style value can never panic the auth path.
func DialectFor(style string) Dialect {
	if d, ok := dialectByName[style]; ok {
		return d
	}
	return shortDialect{}
}

// ParseUplinkTopic parses an uplink topic with the first dialect that
// claims it.
func ParseUplinkTopic(topic string) (Uplink, Dialect, bool) {
	for _, d := range dialects {
		if up, ok := d.ParseUplink(topic); ok {
			return up, d, true
		}
	}
	return Uplink{}, nil, false
}

// ValidStyle reports whether s names a registered dialect.
func ValidStyle(s string) bool {
	_, ok := dialectByName[s]
	return ok
}

// DetectStyle guesses a device's dialect from a Will topic or subscription
// filter; "" when nothing matches.
func DetectStyle(topicOrFilter string) string {
	t := strings.TrimSpace(topicOrFilter)
	if t == "" || strings.HasPrefix(t, "$") {
		return ""
	}
	for _, d := range dialects {
		if d.Detects(t) {
			return d.Name()
		}
	}
	return ""
}

func verifyToken(d *models.Device, password string) (bool, string) {
	if d.AccessTokenHash == "" {
		return false, "no token hash"
	}
	got := auth.HashToken(password)
	if subtle.ConstantTimeCompare([]byte(got), []byte(d.AccessTokenHash)) != 1 {
		return false, "bad token"
	}
	return true, ""
}

const nativeConnectNote = "MQTT username = product.device; password = access token (shown only at create/rotate). TCP :1883 or WebSocket MQTT_WS_ADDR (default :8083)."

// fullDialect and shortDialect share the native credential model (hashed
// token, pk.dn username, cross-publishing between both topic shapes); they
// differ only in uplink topic parsing and downlink addressing.

type fullDialect struct{}

func (fullDialect) Name() string { return StyleFull }

func (fullDialect) Usernames(d *models.Device) []string {
	return []string{d.ProductID + "." + d.DeviceID}
}

func (fullDialect) VerifyPassword(d *models.Device, password, _ string) (bool, string) {
	return verifyToken(d, password)
}

func (fullDialect) CredsUsername(productID, deviceID, _ string) string {
	return productID + "." + deviceID
}

func (fullDialect) UsesSecret() bool { return false }
func (fullDialect) Pinned() bool     { return false }

func (fullDialect) Detects(topicOrFilter string) bool {
	return strings.HasPrefix(topicOrFilter, PrefixFull)
}

func (fullDialect) PublishRoots(d *models.Device) []string {
	return []string{FullUpRoot(d.ProductID, d.DeviceID), PrefixShort}
}

func (fullDialect) SubscribeRoots(d *models.Device) []string {
	return []string{FullDownRoot(d.ProductID, d.DeviceID), PrefixShort}
}

func (fullDialect) ParseUplink(topic string) (Uplink, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) < 6 || !strings.HasPrefix(topic, PrefixFull) || parts[4] != DirUp {
		return Uplink{}, false
	}
	return Uplink{
		ProductID: parts[2],
		DeviceID:  parts[3],
		Kind:      NormalizeCapOp(strings.Join(parts[5:], "/")),
	}, true
}

func (fullDialect) Uplink(kind, aux string, payload []byte) ([]byte, error) {
	return payload, nil
}

func (fullDialect) Downlink(d *models.Device, capability, operation, id string, payload []byte) (string, []byte, error) {
	return DownTopic(StyleFull, d.ProductID, d.DeviceID, capability, operation), payload, nil
}

func (fullDialect) DownTarget(*models.Device) string { return "" }

func (fullDialect) DownViaDevice(*models.Device) bool { return false }

func (fullDialect) Snippet(productID, deviceID, host, credential string) string {
	return fmt.Sprintf(`CogitorCredentials::createAccessToken("%s", "%s", "%s", "%s");`, productID, deviceID, host, credential)
}

func (fullDialect) ConnectNote() string { return nativeConnectNote }

type shortDialect struct{}

func (shortDialect) Name() string { return StyleShort }

func (shortDialect) Usernames(d *models.Device) []string {
	return []string{d.ProductID + "." + d.DeviceID}
}

func (shortDialect) VerifyPassword(d *models.Device, password, _ string) (bool, string) {
	return verifyToken(d, password)
}

func (shortDialect) CredsUsername(productID, deviceID, _ string) string {
	return productID + "." + deviceID
}

func (shortDialect) UsesSecret() bool { return false }
func (shortDialect) Pinned() bool     { return false }

func (shortDialect) Detects(topicOrFilter string) bool {
	return strings.HasPrefix(topicOrFilter, PrefixShort)
}

func (shortDialect) PublishRoots(d *models.Device) []string {
	return []string{FullUpRoot(d.ProductID, d.DeviceID), PrefixShort}
}

func (shortDialect) SubscribeRoots(d *models.Device) []string {
	return []string{FullDownRoot(d.ProductID, d.DeviceID), PrefixShort}
}

// ParseUplink: short topics carry no identity — the device comes from the
// authenticated session.
func (shortDialect) ParseUplink(topic string) (Uplink, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 || !strings.HasPrefix(topic, PrefixShort) {
		return Uplink{}, false
	}
	return Uplink{Kind: NormalizeCapOp(strings.Join(parts[1:], "/"))}, true
}

func (shortDialect) Uplink(kind, aux string, payload []byte) ([]byte, error) {
	return payload, nil
}

func (shortDialect) Downlink(d *models.Device, capability, operation, id string, payload []byte) (string, []byte, error) {
	return DownTopic(StyleShort, d.ProductID, d.DeviceID, capability, operation), payload, nil
}

func (shortDialect) DownTarget(d *models.Device) string {
	return d.ProductID + "." + d.DeviceID
}

func (shortDialect) DownViaDevice(*models.Device) bool { return false }

func (shortDialect) Snippet(productID, deviceID, host, credential string) string {
	return fmt.Sprintf(`CogitorCredentials::createAccessToken("%s", "%s", "%s", "%s");`, productID, deviceID, host, credential)
}

func (shortDialect) ConnectNote() string { return nativeConnectNote }
