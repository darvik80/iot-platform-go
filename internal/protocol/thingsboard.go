package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"cogitor-iot-platform/internal/models"
)

// Topic shapes of the ThingsBoard MQTT device API (v1 JSON payloads).
const (
	tbV1Root            = "v1/devices/me/"
	tbV2Root            = "v2/devices/me/"
	tbTelemetryTopic    = tbV1Root + "telemetry"
	tbAttributesTopic   = tbV1Root + "attributes"
	tbAttrRequestTopic  = tbV1Root + "attributes/request/"
	tbAttrResponseTopic = tbV1Root + "attributes/response/"
	tbRPCRequestV1      = tbV1Root + "rpc/request/"
	tbRPCResponseV1     = tbV1Root + "rpc/response/"
	tbRPCRequestV2      = tbV2Root + "rpc/request/"
	tbRPCResponseV2     = tbV2Root + "rpc/response/"
)

// thingsboardDialect adapts the ThingsBoard MQTT device API: the access
// token is the MQTT username, there is no password, and topics are shared
// device-level shapes (v1/devices/me/…) that carry no identity. RPC v2
// protobuf bodies and v2/fw firmware topics are out of scope.
type thingsboardDialect struct{}

func (thingsboardDialect) Name() string { return StyleThingsBoard }

// Usernames: TB sessions connect with the access token as the MQTT username,
// which the token map resolves — no static username exists.
func (thingsboardDialect) Usernames(*models.Device) []string { return nil }

// VerifyPassword: the username (the access token) already authenticated the
// session; ThingsBoard sends no password.
func (thingsboardDialect) VerifyPassword(*models.Device, string, string) (bool, string) {
	return true, ""
}

// CredsUsername is the raw token, only known at create/rotate time.
func (thingsboardDialect) CredsUsername(_, _, credential string) string {
	return credential
}

func (thingsboardDialect) UsesSecret() bool { return false }

// Pinned: the username is a credential, so TB sessions and payloads must not
// be silently reinterpreted by another dialect.
func (thingsboardDialect) Pinned() bool { return true }

// Detects: never auto-flip a device into this pinned dialect.
func (thingsboardDialect) Detects(string) bool { return false }

func (thingsboardDialect) PublishRoots(*models.Device) []string {
	return []string{tbTelemetryTopic, tbAttributesTopic, tbRPCResponseV1, tbRPCResponseV2}
}

func (thingsboardDialect) SubscribeRoots(*models.Device) []string {
	return []string{tbAttributesTopic, tbRPCRequestV1, tbRPCRequestV2}
}

// ParseUplink claims the exact TB publish shapes. The attributes root also
// matches attribute requests via prefix, so the request id is claimed as aux;
// short devices keep the same kinds (NormalizeCapOp already folds the long
// forms) and ObserveTopicStyle's pinned guard blocks any style flip.
func (thingsboardDialect) ParseUplink(topic string) (Uplink, bool) {
	switch topic {
	case tbTelemetryTopic:
		return Uplink{Kind: KindTelemetry}, true
	case tbAttributesTopic:
		return Uplink{Kind: KindAttrReport}, true
	}
	if rest, ok := strings.CutPrefix(topic, tbAttrRequestTopic); ok && tbID(rest) {
		return Uplink{Kind: KindAttrRequest, Aux: rest}, true
	}
	for _, prefix := range []string{tbRPCResponseV1, tbRPCResponseV2} {
		if rest, ok := strings.CutPrefix(topic, prefix); ok && tbID(rest) {
			return Uplink{Kind: KindRPCResponse, Aux: rest}, true
		}
	}
	return Uplink{}, false
}

// tbID rejects empty ids, wildcards, and nested segments.
func tbID(s string) bool {
	return s != "" && !strings.ContainsAny(s, "/+#")
}

func (thingsboardDialect) Uplink(kind, aux string, payload []byte) ([]byte, error) {
	switch kind {
	case KindTelemetry, KindAttrReport:
		return payload, nil
	case KindAttrRequest:
		return tbAttrRequestToInternal(aux, payload)
	case KindRPCResponse:
		return tbRPCResponseToInternal(aux, payload)
	default:
		return nil, fmt.Errorf("thingsboard: unsupported uplink kind %q", kind)
	}
}

func (thingsboardDialect) Downlink(_ *models.Device, capability, operation, id string, payload []byte) (string, []byte, error) {
	switch capability + "/" + operation {
	case CapAttributes + "/" + OpUpdate:
		return tbAttributesTopic, payload, nil
	case CapAttributes + "/" + OpResponse:
		return tbAttrResponse(id, payload)
	case CapRPC + "/" + OpRequest:
		return tbRPCRequest(id, payload)
	default:
		return "", nil, fmt.Errorf("thingsboard: unsupported downlink %s/%s", capability, operation)
	}
}

// DownViaDevice: topics are shared across every device, so a broadcast would
// leak to other accounts; delivery must resolve the device's sessions.
func (thingsboardDialect) DownViaDevice(*models.Device) bool { return true }

// DownTarget: no static username — delivery goes through the token map.
func (thingsboardDialect) DownTarget(*models.Device) string { return "" }

func (thingsboardDialect) Snippet(_, _, host, credential string) string {
	return fmt.Sprintf("host: %s\nusername: %s (access token)\ntelemetry: v1/devices/me/telemetry\nattributes: v1/devices/me/attributes", host, credential)
}

func (thingsboardDialect) ConnectNote() string {
	return "ThingsBoard style: MQTT username = device access token, no password. Publish v1/devices/me/telemetry and v1/devices/me/attributes; request attributes via v1/devices/me/attributes/request/$id; subscribe v1/devices/me/attributes and v1/devices/me/rpc/request/+; answer RPC on v1/devices/me/rpc/response/$id."
}

// tbAttrRequestToInternal maps a TB attribute request to the internal shape.
// The TB request id lives in the topic, so it is injected as the payload id
// the internal request/response correlation echoes back. sharedKeys ask for
// server-side attributes (internal desired), clientKeys for the device's own
// reported ones.
func tbAttrRequestToInternal(aux string, payload []byte) ([]byte, error) {
	var probe struct {
		Reported []string `json:"reported"`
		Desired  []string `json:"desired"`
	}
	if err := json.Unmarshal(payload, &probe); err == nil && (probe.Reported != nil || probe.Desired != nil) {
		return payload, nil // internal-shaped payload on a TB-shaped topic
	}
	var env struct {
		SharedKeys json.RawMessage `json:"sharedKeys"`
		ClientKeys json.RawMessage `json:"clientKeys"`
	}
	_ = json.Unmarshal(payload, &env) // tolerate non-object payloads
	out := map[string]any{"id": aux}
	reported := tbKeyList(env.ClientKeys)
	desired := tbKeyList(env.SharedKeys)
	if reported == nil && desired == nil {
		desired = []string{"*"}
	}
	if reported != nil {
		out["reported"] = reported
	}
	if desired != nil {
		out["desired"] = desired
	}
	return json.Marshal(out)
}

// tbKeyList accepts a TB key list as JSON array or comma-separated string.
func tbKeyList(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return tbCompactKeys(arr)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return tbCompactKeys(strings.Split(s, ","))
	}
	return nil
}

func tbCompactKeys(list []string) []string {
	out := make([]string, 0, len(list))
	for _, k := range list {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// tbRPCResponseToInternal wraps an arbitrary TB RPC response payload; the
// request id lives in the topic, so it becomes the internal payload id.
func tbRPCResponseToInternal(aux string, payload []byte) ([]byte, error) {
	var probe struct {
		ID   json.RawMessage `json:"id"`
		Code *int            `json:"code"`
	}
	if err := json.Unmarshal(payload, &probe); err == nil && len(probe.ID) > 0 && probe.Code != nil {
		return payload, nil // internal-shaped payload on a TB-shaped topic
	}
	var data json.RawMessage
	if len(payload) > 0 {
		data = payload
	}
	var id any = aux
	if n, err := strconv.ParseUint(aux, 10, 64); err == nil {
		id = n
	}
	return json.Marshal(map[string]any{"id": id, "code": 0, "data": data})
}

// tbAttrResponse answers v1/devices/me/attributes/request/$id: the internal
// payload's id seeds the response topic; reported/desired become TB's
// client/shared maps.
func tbAttrResponse(id string, payload []byte) (string, []byte, error) {
	var env struct {
		ID       json.RawMessage    `json:"id"`
		Reported map[string]any     `json:"reported"`
		Desired  map[string]any     `json:"desired"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return "", nil, err
	}
	tid := tbRawIDString(env.ID, id)
	if tid == "" {
		return "", nil, errors.New("thingsboard: attribute response without id")
	}
	out := map[string]any{}
	if env.Reported != nil {
		out["client"] = env.Reported
	}
	if env.Desired != nil {
		out["shared"] = env.Desired
	}
	body, err := json.Marshal(out)
	if err != nil {
		return "", nil, err
	}
	return tbAttrResponseTopic + tid, body, nil
}

// tbRawIDString extracts a topic id from a JSON id value of any scalar type.
func tbRawIDString(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return fallback
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return fallback
}

// tbRPCRequest builds a v1 RPC request; the internal payload's numeric id
// seeds the topic so the device's v1/devices/me/rpc/response/$id reply
// correlates. TB v1 RPC bodies are plain JSON ({"method","params"}).
func tbRPCRequest(id string, payload []byte) (string, []byte, error) {
	var req struct {
		ID     uint32 `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", nil, err
	}
	if req.Method == "" {
		return "", nil, errors.New("thingsboard: rpc downlink without method")
	}
	if req.ID != 0 {
		id = strconv.FormatUint(uint64(req.ID), 10)
	} else if id == "" {
		return "", nil, errors.New("thingsboard: rpc downlink without id")
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{"method": req.Method, "params": req.Params})
	if err != nil {
		return "", nil, err
	}
	return tbRPCRequestV1 + id, body, nil
}
