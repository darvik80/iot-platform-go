// Alink dialect: topics, auth, envelopes, and payload translation for devices
// built against the Alibaba Cloud IoT (Alink) protocol. Shapes follow the
// embed-framework alicloud_iot component; the firmware requires STRING ids
// in downlink envelopes and publishes replies with a numeric code where
// 200 means success.
package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cogitor-iot-platform/internal/models"
)

// AlinkDefaultTimestamp is the fixed timestamp the firmware embeds in its
// clientId KV section (2050-01-01T00:00:00Z) and HMAC sign input.
const AlinkDefaultTimestamp = "2524608000000"

// AlinkUsername returns the MQTT username of an alink device: {deviceName}&{productKey}.
func AlinkUsername(productID, deviceID string) string {
	return deviceID + "&" + productID
}

// AlinkSign computes the alink MQTT password: uppercase-hex HMAC-SHA256 of
// clientId{username}deviceName{dn}productKey{pk}timestamp{ts}, keyed by the device secret.
func AlinkSign(deviceSecret, productID, deviceID, timestamp string) string {
	if timestamp == "" {
		timestamp = AlinkDefaultTimestamp
	}
	username := AlinkUsername(productID, deviceID)
	data := "clientId" + username + "deviceName" + deviceID + "productKey" + productID + "timestamp" + timestamp
	mac := hmac.New(sha256.New, []byte(deviceSecret))
	mac.Write([]byte(data))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}

// AlinkVerifyAuth validates a CONNECT password for an alink device. The sign
// timestamp comes from the clientId |timestamp=…| KV section, falling back to
// the firmware default.
func AlinkVerifyAuth(password, deviceSecret, productID, deviceID, clientID string) bool {
	if deviceSecret == "" || password == "" {
		return false
	}
	want := AlinkSign(deviceSecret, productID, deviceID, AlinkTimestampFromClientID(clientID))
	return subtle.ConstantTimeCompare([]byte(want), []byte(strings.ToUpper(strings.TrimSpace(password)))) == 1
}

// AlinkTimestampFromClientID extracts the timestamp=… KV value from an alink
// clientId such as "dev&pk|timestamp=2524608000000,ext=3,…|".
func AlinkTimestampFromClientID(clientID string) string {
	const marker = "timestamp="
	i := strings.Index(clientID, marker)
	if i < 0 {
		return AlinkDefaultTimestamp
	}
	rest := clientID[i+len(marker):]
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j == 0 {
		return AlinkDefaultTimestamp
	}
	return rest[:j]
}

// AlinkSysTopic builds /sys/{pk}/{dn}/{tail}.
func AlinkSysTopic(productID, deviceID, tail string) string {
	return "/sys/" + productID + "/" + deviceID + "/" + tail
}

// AlinkNTPTopic builds /ext/ntp/{pk}/{dn}/{suffix}.
func AlinkNTPTopic(productID, deviceID, suffix string) string {
	return "/ext/ntp/" + productID + "/" + deviceID + "/" + suffix
}

// AlinkOTATopic builds /ota/device/{suffix}/{pk}/{dn}.
func AlinkOTATopic(suffix, productID, deviceID string) string {
	return "/ota/device/" + suffix + "/" + productID + "/" + deviceID
}

// AlinkUplinkRoots lists topic roots an alink device may publish under.
func AlinkUplinkRoots(productID, deviceID string) []string {
	return []string{
		AlinkSysTopic(productID, deviceID, ""),
		AlinkNTPTopic(productID, deviceID, ""),
		AlinkOTATopic("inform", productID, deviceID),
		AlinkOTATopic("progress", productID, deviceID),
	}
}

// AlinkDownlinkRoots lists topic roots an alink device may subscribe to.
func AlinkDownlinkRoots(productID, deviceID string) []string {
	return []string{
		AlinkSysTopic(productID, deviceID, ""),
		AlinkNTPTopic(productID, deviceID, ""),
		AlinkOTATopic("upgrade", productID, deviceID),
	}
}

// AlinkTopic is a parsed alink uplink topic.
type AlinkTopic struct {
	ProductID string
	DeviceID  string
	Kind      string // internal ingest kind
	Aux       string // event id or service method, when present
}

// AlinkParseTopic parses an alink uplink topic into device identity and
// internal kind. ok is false for non-alink topics and unrecognized tails.
func AlinkParseTopic(topic string) (AlinkTopic, bool) {
	t := strings.TrimPrefix(strings.TrimSpace(topic), "/")
	switch {
	case strings.HasPrefix(t, "sys/"):
		parts := strings.Split(t, "/")
		if len(parts) < 4 || !alinkID(parts[1]) || !alinkID(parts[2]) {
			return AlinkTopic{}, false
		}
		at := AlinkTopic{ProductID: parts[1], DeviceID: parts[2]}
		tail := strings.Join(parts[3:], "/")
		switch tail {
		case "thing/event/property/post":
			at.Kind = KindTelemetry
			return at, true
		case "thing/property/desired/get":
			at.Kind = KindAttrGet
			return at, true
		case "thing/property/desired/delete":
			at.Kind = KindAttrDelete
			return at, true
		case "thing/ota/firmware/get":
			at.Kind = KindOTAQuery
			return at, true
		case "thing/service/property/set_reply":
			at.Kind, at.Aux = KindRPCResponse, "property/set"
			return at, true
		}
		if eid, ok := cutAffix(tail, "thing/event/", "/post"); ok && eid != "" {
			at.Kind, at.Aux = KindEvent, eid
			return at, true
		}
		if m, ok := cutAffix(tail, "thing/service/", "/_reply"); ok && m != "" {
			at.Kind, at.Aux = KindRPCResponse, m
			return at, true
		}
		return AlinkTopic{}, false
	case strings.HasPrefix(t, "ext/ntp/"):
		parts := strings.Split(t, "/") // ext, ntp, pk, dn, suffix
		if len(parts) != 5 || parts[4] != "request" || !alinkID(parts[2]) || !alinkID(parts[3]) {
			return AlinkTopic{}, false
		}
		return AlinkTopic{ProductID: parts[2], DeviceID: parts[3], Kind: KindNTPRequest}, true
	case strings.HasPrefix(t, "ota/device/"):
		parts := strings.Split(t, "/") // ota, device, suffix, pk, dn
		if len(parts) != 5 || !alinkID(parts[3]) || !alinkID(parts[4]) {
			return AlinkTopic{}, false
		}
		at := AlinkTopic{ProductID: parts[3], DeviceID: parts[4]}
		switch parts[2] {
		case "inform":
			at.Kind = KindOTAVersion
		case "progress":
			at.Kind = KindOTAProgress
		default:
			return AlinkTopic{}, false
		}
		return at, true
	}
	return AlinkTopic{}, false
}

// alinkID rejects empty identities and MQTT wildcards.
func alinkID(s string) bool {
	return s != "" && !strings.ContainsAny(s, "+#")
}

func cutAffix(s, prefix, suffix string) (string, bool) {
	rest, ok := strings.CutPrefix(s, prefix)
	if !ok {
		return "", false
	}
	out, ok := strings.CutSuffix(rest, suffix)
	if !ok {
		return "", false
	}
	return out, true
}

// AlinkToInternal converts an alink uplink payload into the internal v1
// payload for the parsed kind. aux carries the event id or service method.
func AlinkToInternal(kind, aux string, payload []byte) ([]byte, error) {
	switch kind {
	case KindTelemetry:
		return alinkTelemetryToInternal(payload)
	case KindEvent:
		return alinkEventToInternal(aux, payload)
	case KindRPCResponse:
		return alinkRPCReplyToInternal(payload)
	case KindAttrGet:
		return alinkDesiredGetToInternal(payload)
	case KindAttrDelete:
		return alinkDesiredDeleteToInternal(payload)
	case KindNTPRequest:
		return alinkNTPToInternal(payload)
	case KindOTAQuery:
		return alinkOTAParamsToInternal(payload)
	case KindOTAVersion:
		return alinkOTAVersionToInternal(payload)
	case KindOTAProgress:
		return alinkOTAProgressToInternal(payload)
	default:
		return nil, fmt.Errorf("alink: unsupported uplink kind %q", kind)
	}
}

func alinkTelemetryToInternal(payload []byte) ([]byte, error) {
	var env struct {
		Params map[string]json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(env.Params))
	for k := range env.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	groups := map[int64]map[string]any{}
	untimed := map[string]any{}
	for _, k := range keys {
		v, ts, ok := alinkPropValue(env.Params[k])
		if !ok {
			continue
		}
		if ts > 0 {
			if groups[ts] == nil {
				groups[ts] = map[string]any{}
			}
			groups[ts][k] = v
		} else {
			untimed[k] = v
		}
	}
	times := make([]int64, 0, len(groups))
	for ts := range groups {
		times = append(times, ts)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	out := make([]map[string]any, 0, len(times)+1)
	for _, ts := range times {
		out = append(out, map[string]any{"ts": ts, "values": groups[ts]})
	}
	if len(untimed) > 0 {
		out = append(out, map[string]any{"values": untimed})
	}
	return json.Marshal(out)
}

// alinkPropValue unwraps an alink property entry {value, time?}; a plain
// value (no wrapper) is accepted with ts=0.
func alinkPropValue(raw json.RawMessage) (value any, ts int64, ok bool) {
	var prop struct {
		Value json.RawMessage `json:"value"`
		Time  int64           `json:"time"`
	}
	if err := json.Unmarshal(raw, &prop); err == nil && len(prop.Value) > 0 {
		if err := json.Unmarshal(prop.Value, &value); err != nil {
			return nil, 0, false
		}
		return value, prop.Time, true
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, 0, false
	}
	return value, 0, true
}

func alinkEventToInternal(aux string, payload []byte) ([]byte, error) {
	var env struct {
		ID     string `json:"id"`
		Params struct {
			Value map[string]any `json:"value"`
			Time  int64          `json:"time"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	out := map[string]any{"id": env.ID, "type": aux}
	if env.Params.Time > 0 {
		out["ts"] = env.Params.Time
	}
	if env.Params.Value != nil {
		out["data"] = env.Params.Value
	}
	return json.Marshal(out)
}

// alinkRPCReplyToInternal maps the alink success code 200 to the internal 0.
func alinkRPCReplyToInternal(payload []byte) ([]byte, error) {
	var env struct {
		ID      json.RawMessage `json:"id"`
		Code    *int            `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	code := 0
	if env.Code != nil && *env.Code != 200 {
		code = *env.Code
	}
	out := map[string]any{"id": env.ID, "code": code, "message": env.Message}
	if len(env.Data) > 0 {
		out["data"] = env.Data
	}
	return json.Marshal(out)
}

func alinkDesiredGetToInternal(payload []byte) ([]byte, error) {
	var env struct {
		ID     json.RawMessage `json:"id"`
		Params []string        `json:"params"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	if env.Params == nil {
		env.Params = []string{}
	}
	return json.Marshal(map[string]any{"id": env.ID, "keys": env.Params})
}

func alinkDesiredDeleteToInternal(payload []byte) ([]byte, error) {
	var env struct {
		ID     json.RawMessage            `json:"id"`
		Params map[string]json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(env.Params))
	for k := range env.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return json.Marshal(map[string]any{"id": env.ID, "keys": keys})
}

// alinkNTPToInternal converts the string deviceSendTime the firmware sends
// into the number the internal NTP handler expects.
func alinkNTPToInternal(payload []byte) ([]byte, error) {
	var env struct {
		DeviceSendTime json.RawMessage `json:"deviceSendTime"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	ts, _ := alinkInt(env.DeviceSendTime)
	return json.Marshal(map[string]any{"deviceSendTime": ts})
}

// alinkInt accepts a JSON number or numeric string.
func alinkInt(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			return v, true
		}
	}
	return 0, false
}

func alinkOTAParamsToInternal(payload []byte) ([]byte, error) {
	var env struct {
		Params struct {
			Module string `json:"module"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"module": env.Params.Module})
}

func alinkOTAVersionToInternal(payload []byte) ([]byte, error) {
	var env struct {
		Params struct {
			Version string `json:"version"`
			Module  string `json:"module"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"version": env.Params.Version, "module": env.Params.Module})
}

// alinkOTAProgressToInternal converts the string step the firmware reports
// into the number the internal OTA progress handler expects.
func alinkOTAProgressToInternal(payload []byte) ([]byte, error) {
	var env struct {
		Params struct {
			Step   json.RawMessage `json:"step"`
			Desc   string          `json:"desc"`
			Module string          `json:"module"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	step, _ := alinkInt(env.Params.Step)
	return json.Marshal(map[string]any{"module": env.Params.Module, "step": step, "desc": env.Params.Desc})
}

// AlinkDownlink translates an internal downlink into the alink topic and
// payload. id seeds alink message ids for downlinks whose internal payload
// carries none (property set; RPC falls back to it when its id is 0).
func AlinkDownlink(productID, deviceID, capability, operation, id string, payload []byte) (string, []byte, error) {
	switch capability + "/" + operation {
	case CapRPC + "/" + OpRequest:
		return alinkRPCDownlink(productID, deviceID, id, payload)
	case CapAttributes + "/" + OpUpdate:
		return alinkPropertySet(productID, deviceID, id, payload)
	case CapAttributes + "/" + OpGetReply:
		return alinkDesiredReply(productID, deviceID, "property/desired/get_reply", payload)
	case CapAttributes + "/" + OpDeleteReply:
		return alinkDesiredReply(productID, deviceID, "property/desired/delete_reply", payload)
	case CapNTP + "/" + OpResponse:
		return alinkNTPResponse(productID, deviceID, payload)
	case CapOTA + "/" + OpUpdate:
		return alinkOTAUpgrade(productID, deviceID, payload, "1000")
	case CapOTA + "/" + OpCancel:
		return alinkOTAUpgrade(productID, deviceID, payload, "-1")
	default:
		return "", nil, fmt.Errorf("alink: unsupported downlink %s/%s", capability, operation)
	}
}

func alinkRPCDownlink(productID, deviceID, id string, payload []byte) (string, []byte, error) {
	var req struct {
		ID     uint32 `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", nil, err
	}
	if req.Method == "" {
		return "", nil, errors.New("alink: rpc downlink without method")
	}
	if req.ID != 0 {
		id = strconv.FormatUint(uint64(req.ID), 10)
	} else if id == "" {
		return "", nil, errors.New("alink: rpc downlink without id")
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{
		"id": id, "version": "1.0",
		"method": "thing.service." + req.Method, "params": req.Params,
	})
	if err != nil {
		return "", nil, err
	}
	return AlinkSysTopic(productID, deviceID, "thing/service/"+req.Method), body, nil
}

// alinkPropertySet stringifies every param value: the firmware reads property
// set params as strings only and silently drops the rest.
func alinkPropertySet(productID, deviceID, id string, payload []byte) (string, []byte, error) {
	if id == "" {
		return "", nil, errors.New("alink: property set without id")
	}
	var attrs map[string]any
	if err := json.Unmarshal(payload, &attrs); err != nil {
		return "", nil, err
	}
	params := make(map[string]string, len(attrs))
	for k, v := range attrs {
		params[k] = alinkParamString(v)
	}
	body, err := json.Marshal(map[string]any{
		"id": id, "version": "1.0", "method": "thing.service.property.set",
		"params": params,
	})
	if err != nil {
		return "", nil, err
	}
	return AlinkSysTopic(productID, deviceID, "thing/service/property/set"), body, nil
}

func alinkParamString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// alinkDesiredReply echoes the request id (raw passthrough — the firmware
// requires a STRING id) and the desired attributes from the internal payload.
func alinkDesiredReply(productID, deviceID, tail string, payload []byte) (string, []byte, error) {
	var env struct {
		ID      json.RawMessage `json:"id"`
		Desired map[string]any  `json:"desired"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return "", nil, err
	}
	if len(env.ID) == 0 {
		return "", nil, errors.New("alink: desired reply without id")
	}
	out := map[string]any{"id": env.ID, "code": 200, "message": "success"}
	if env.Desired != nil {
		out["data"] = env.Desired
	}
	body, err := json.Marshal(out)
	if err != nil {
		return "", nil, err
	}
	return AlinkSysTopic(productID, deviceID, "thing/"+tail), body, nil
}

func alinkNTPResponse(productID, deviceID string, payload []byte) (string, []byte, error) {
	var env struct {
		DeviceSendTime int64 `json:"deviceSendTime"`
		ServerRecvTime int64 `json:"serverRecvTime"`
		ServerSendTime int64 `json:"serverSendTime"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return "", nil, err
	}
	body, err := json.Marshal(map[string]any{
		"deviceSendTime": env.DeviceSendTime,
		"serverRecvTime": env.ServerRecvTime,
		"serverSendTime": env.ServerSendTime,
	})
	if err != nil {
		return "", nil, err
	}
	return AlinkNTPTopic(productID, deviceID, "response"), body, nil
}

// alinkOTAUpgrade wraps the internal OTA body in the alink upgrade envelope.
// The firmware only proceeds on the STRING code "1000"; anything else (here
// "-1" for cancel) is ignored. The firmware verifies md5 only when present;
// the platform carries sha256, so verification stays off.
func alinkOTAUpgrade(productID, deviceID string, payload []byte, code string) (string, []byte, error) {
	var data json.RawMessage
	if err := json.Unmarshal(payload, &data); err != nil {
		return "", nil, err
	}
	body, err := json.Marshal(map[string]any{"code": code, "data": data})
	if err != nil {
		return "", nil, err
	}
	return AlinkOTATopic("upgrade", productID, deviceID), body, nil
}

// alinkDialect adapts the Alibaba Cloud IoT wire contract. Credential model:
// raw device secret (HMAC key) instead of a hashed token, and a second
// username {device}&{product}.
type alinkDialect struct{}

func (alinkDialect) Name() string { return StyleAlink }

func (alinkDialect) Usernames(d *models.Device) []string {
	return []string{
		d.ProductID + "." + d.DeviceID,
		AlinkUsername(d.ProductID, d.DeviceID),
	}
}

func (alinkDialect) VerifyPassword(d *models.Device, password, clientID string) (bool, string) {
	if !AlinkVerifyAuth(password, d.AlinkSecret, d.ProductID, d.DeviceID, clientID) {
		return false, "bad alink signature"
	}
	return true, ""
}

func (alinkDialect) CredsUsername(productID, deviceID, _ string) string {
	return AlinkUsername(productID, deviceID)
}

func (alinkDialect) UsesSecret() bool { return true }
func (alinkDialect) Pinned() bool     { return true }

func (alinkDialect) Detects(topicOrFilter string) bool {
	t := strings.TrimPrefix(topicOrFilter, "/")
	return strings.HasPrefix(t, "sys/") ||
		strings.HasPrefix(t, "ext/ntp/") ||
		strings.HasPrefix(t, "ota/device/")
}

func (alinkDialect) PublishRoots(d *models.Device) []string {
	return AlinkUplinkRoots(d.ProductID, d.DeviceID)
}

func (alinkDialect) SubscribeRoots(d *models.Device) []string {
	return append(AlinkUplinkRoots(d.ProductID, d.DeviceID), AlinkDownlinkRoots(d.ProductID, d.DeviceID)...)
}

func (alinkDialect) ParseUplink(topic string) (Uplink, bool) {
	at, ok := AlinkParseTopic(topic)
	if !ok {
		return Uplink{}, false
	}
	return Uplink{ProductID: at.ProductID, DeviceID: at.DeviceID, Kind: at.Kind, Aux: at.Aux}, true
}

func (alinkDialect) Uplink(kind, aux string, payload []byte) ([]byte, error) {
	return AlinkToInternal(kind, aux, payload)
}

func (alinkDialect) Downlink(d *models.Device, capability, operation, id string, payload []byte) (string, []byte, error) {
	return AlinkDownlink(d.ProductID, d.DeviceID, capability, operation, id, payload)
}

func (alinkDialect) DownTarget(d *models.Device) string {
	return AlinkUsername(d.ProductID, d.DeviceID)
}

func (alinkDialect) DownViaDevice(*models.Device) bool { return false }

func (alinkDialect) Snippet(productID, deviceID, host, credential string) string {
	return fmt.Sprintf("host=%s\nproductKey=%s\ndeviceName=%s\ndeviceSecret=%s", host, productID, deviceID, credential)
}

func (alinkDialect) ConnectNote() string {
	return "Alink style: firmware derives username {device}&{product}, client_id and HMAC-SHA256 password from productKey/deviceName/deviceSecret. Secret shown only at create/rotate."
}
