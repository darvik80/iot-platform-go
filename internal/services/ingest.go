package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"crearts-iot-platform/internal/db"
	"crearts-iot-platform/internal/models"
)

func (p *Platform) StoreTelemetry(d *models.Device, payload []byte) error {
	if p.SeenRecently(d.ID, "telemetry", payload, 2*time.Second) {
		return nil
	}
	now := db.NowMS()
	samples, err := parseTelemetry(payload, now)
	if err != nil {
		return err
	}
	tx, err := p.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keys := map[string]any{}
	for _, s := range samples {
		var vn any
		var vt any
		var vb any
		if s.ValueNum != nil {
			vn = *s.ValueNum
			keys[s.Key] = *s.ValueNum
		}
		if s.ValueText != nil {
			vt = *s.ValueText
			keys[s.Key] = *s.ValueText
		}
		if s.ValueBool != nil {
			b := 0
			if *s.ValueBool {
				b = 1
			}
			vb = b
			keys[s.Key] = *s.ValueBool
		}
		if _, err := tx.Exec(`INSERT INTO telemetry_samples(device_pk, key, value_num, value_text, value_bool, ts, ingested_at) VALUES(?,?,?,?,?,?,?)`,
			d.ID, s.Key, vn, vt, vb, s.TS, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.TouchSeen(d)
	p.emit("telemetry", map[string]any{"device_id": d.ID, "product_id": d.ProductID, "device": d.DeviceID, "keys": keys, "ts": now})
	p.publishRules(d.ProductID, d.DeviceID, "telemetry", map[string]any{
		"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "keys": keys,
	})
	return nil
}

func parseTelemetry(payload []byte, now int64) ([]models.TelemetrySample, error) {
	var arr []map[string]any
	if err := json.Unmarshal(payload, &arr); err == nil {
		var out []models.TelemetrySample
		for _, item := range arr {
			ts := now
			if v, ok := item["ts"].(float64); ok {
				ts = int64(v)
			}
			values, _ := item["values"].(map[string]any)
			out = append(out, kvSamples(values, ts)...)
		}
		return out, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, err
	}
	if values, ok := obj["values"].(map[string]any); ok {
		ts := now
		if v, ok := obj["ts"].(float64); ok {
			ts = int64(v)
		}
		return kvSamples(values, ts), nil
	}
	return kvSamples(obj, now), nil
}

func kvSamples(m map[string]any, ts int64) []models.TelemetrySample {
	var out []models.TelemetrySample
	for k, v := range m {
		if k == "ts" || k == "values" {
			continue
		}
		s := models.TelemetrySample{Key: k, TS: ts}
		switch t := v.(type) {
		case float64:
			s.ValueNum = &t
		case bool:
			s.ValueBool = &t
		case string:
			s.ValueText = &t
		default:
			b, _ := json.Marshal(t)
			str := string(b)
			s.ValueText = &str
		}
		out = append(out, s)
	}
	return out
}

func (p *Platform) QueryTelemetry(devicePK int64, keys []string, from, to int64, limit int) ([]models.TelemetrySample, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	if to == 0 {
		to = db.NowMS()
	}
	query := `SELECT key, value_num, value_text, value_bool, ts FROM telemetry_samples WHERE device_pk=? AND ts>=? AND ts<=?`
	args := []any{devicePK, from, to}
	if len(keys) > 0 {
		query += ` AND key IN (`
		for i, k := range keys {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, k)
		}
		query += `)`
	}
	query += ` ORDER BY ts ASC LIMIT ?`
	args = append(args, limit)
	rows, err := p.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.TelemetrySample
	for rows.Next() {
		var s models.TelemetrySample
		var vn sql.NullFloat64
		var vt sql.NullString
		var vb sql.NullInt64
		if err := rows.Scan(&s.Key, &vn, &vt, &vb, &s.TS); err != nil {
			return nil, err
		}
		if vn.Valid {
			s.ValueNum = &vn.Float64
		}
		if vt.Valid {
			s.ValueText = &vt.String
		}
		if vb.Valid {
			b := vb.Int64 == 1
			s.ValueBool = &b
		}
		out = append(out, s)
	}
	if out == nil {
		out = []models.TelemetrySample{}
	}
	return out, nil
}

func (p *Platform) LatestTelemetry(devicePK int64) (map[string]models.TelemetrySample, error) {
	rows, err := p.DB.Query(`
		SELECT t.key, t.value_num, t.value_text, t.value_bool, t.ts
		FROM telemetry_samples t
		INNER JOIN (
			SELECT key, MAX(ts) AS mts FROM telemetry_samples WHERE device_pk=? GROUP BY key
		) x ON x.key = t.key AND x.mts = t.ts
		WHERE t.device_pk=?`, devicePK, devicePK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.TelemetrySample{}
	for rows.Next() {
		var s models.TelemetrySample
		var vn sql.NullFloat64
		var vt sql.NullString
		var vb sql.NullInt64
		if err := rows.Scan(&s.Key, &vn, &vt, &vb, &s.TS); err != nil {
			return nil, err
		}
		if vn.Valid {
			s.ValueNum = &vn.Float64
		}
		if vt.Valid {
			s.ValueText = &vt.String
		}
		if vb.Valid {
			b := vb.Int64 == 1
			s.ValueBool = &b
		}
		out[s.Key] = s
	}
	return out, nil
}

func (p *Platform) GetAttributes(devicePK int64) (reported, desired map[string]any, err error) {
	reported, err = p.loadAttrs("device_attributes_reported", devicePK)
	if err != nil {
		return
	}
	desired, err = p.loadAttrs("device_attributes_desired", devicePK)
	return
}

func (p *Platform) loadAttrs(table string, devicePK int64) (map[string]any, error) {
	rows, err := p.DB.Query(fmt.Sprintf(`SELECT key, value_json FROM %s WHERE device_pk=?`, table), devicePK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			parsed = v
		}
		out[k] = parsed
	}
	return out, nil
}

func (p *Platform) MergeReported(d *models.Device, attrs map[string]any) error {
	now := db.NowMS()
	tx, err := p.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range attrs {
		vj := jsonValue(v)
		if _, err := tx.Exec(`INSERT INTO device_attributes_reported(device_pk, key, value_json, updated_at) VALUES(?,?,?,?)
			ON CONFLICT(device_pk, key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at`,
			d.ID, k, vj, now); err != nil {
			return err
		}
		_, _ = tx.Exec(`INSERT INTO attribute_history(device_pk, scope, key, value_json, updated_at) VALUES(?,?,?,?,?)`,
			d.ID, "reported", k, vj, now)
		if k == "firmwareVersion" {
			if s, ok := v.(string); ok {
				_, _ = tx.Exec(`UPDATE devices SET fw_version=?, updated_at=? WHERE id=?`, s, now, d.ID)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.TouchSeen(d)
	p.emit("attributes.reported", map[string]any{"device_id": d.ID, "attrs": attrs})
	p.publishRules(d.ProductID, d.DeviceID, "attributes/reported", map[string]any{
		"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "attrs": attrs,
	})
	return nil
}

func (p *Platform) SetDesired(d *models.Device, attrs map[string]any, publish bool) error {
	now := db.NowMS()
	tx, err := p.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range attrs {
		vj := jsonValue(v)
		if _, err := tx.Exec(`INSERT INTO device_attributes_desired(device_pk, key, value_json, updated_at) VALUES(?,?,?,?)
			ON CONFLICT(device_pk, key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at`,
			d.ID, k, vj, now); err != nil {
			return err
		}
		_, _ = tx.Exec(`INSERT INTO attribute_history(device_pk, scope, key, value_json, updated_at) VALUES(?,?,?,?,?)`,
			d.ID, "desired", k, vj, now)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if publish && p.Pub != nil {
		b, _ := json.Marshal(attrs)
		topic := p.DownTopic(d, "attributes", "update")
		_ = p.Pub.Publish(topic, 1, false, b)
	}
	p.emit("attributes.desired", map[string]any{"device_id": d.ID, "attrs": attrs})
	p.publishRules(d.ProductID, d.DeviceID, "attributes/desired", map[string]any{
		"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "attrs": attrs,
	})
	return nil
}

func (p *Platform) ReplyAttributeRequest(d *models.Device, payload []byte) error {
	var req struct {
		ID       uint32   `json:"id"`
		Reported []string `json:"reported"`
		Desired  []string `json:"desired"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return err
	}
	reported, desired, err := p.GetAttributes(d.ID)
	if err != nil {
		return err
	}
	resp := map[string]any{"id": req.ID}
	if req.Reported != nil {
		resp["reported"] = filterKeys(reported, req.Reported)
	}
	if req.Desired != nil {
		resp["desired"] = filterKeys(desired, req.Desired)
	}
	b, _ := json.Marshal(resp)
	if p.Pub == nil {
		return errors.New("mqtt not connected")
	}
	return p.Pub.Publish(p.DownTopic(d, "attributes", "response"), 1, false, b)
}

func filterKeys(src map[string]any, keys []string) map[string]any {
	if len(keys) == 1 && keys[0] == "*" {
		return src
	}
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := src[k]; ok {
			out[k] = v
		}
	}
	return out
}

type RPCRequest struct {
	Method        string         `json:"method"`
	Params        map[string]any `json:"params"`
	TimeoutMS     int            `json:"timeout_ms"`
	CorrelationID string         `json:"correlation_id"`
}

func (p *Platform) InvokeRPC(d *models.Device, req RPCRequest, wait bool) (*models.RPCCall, error) {
	if req.Method == "" {
		return nil, errors.New("method required")
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	timeout := p.Cfg.RPCDefaultTimeout
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	rid := p.nextRPCID()
	now := db.NowMS()
	paramsJSON := jsonValue(req.Params)
	res, err := p.DB.Exec(`INSERT INTO rpc_calls(device_pk, request_id, method, params_json, status, correlation_id, created_at) VALUES(?,?,?,?, 'pending', ?,?)`,
		d.ID, rid, req.Method, paramsJSON, req.CorrelationID, now)
	if err != nil {
		return nil, err
	}
	pk, _ := res.LastInsertId()
	body := map[string]any{"id": rid, "method": req.Method, "params": req.Params}
	b, _ := json.Marshal(body)
	if p.Pub == nil {
		return nil, errors.New("mqtt not connected")
	}
	if err := p.Pub.Publish(p.DownTopic(d, "rpc", "request"), 1, false, b); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%d:%d", d.ID, rid)
	ch := make(chan models.RPCCall, 1)
	p.mu.Lock()
	p.rpcWaiters[key] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.rpcWaiters, key)
		p.mu.Unlock()
	}()

	call := models.RPCCall{ID: pk, DevicePK: d.ID, RequestID: rid, Method: req.Method, ParamsJSON: paramsJSON, Status: "pending", CorrelationID: req.CorrelationID, CreatedAt: now}
	if !wait {
		return &call, nil
	}
	select {
	case done := <-ch:
		return &done, nil
	case <-time.After(timeout):
		_, _ = p.DB.Exec(`UPDATE rpc_calls SET status='timeout', code=408, message='timeout', completed_at=? WHERE id=?`, db.NowMS(), pk)
		call.Status = "timeout"
		code := 408
		call.Code = &code
		call.Message = "timeout"
		completed := db.NowMS()
		call.CompletedAt = &completed
		return &call, nil
	}
}

func (p *Platform) CompleteRPC(d *models.Device, payload []byte) error {
	var resp struct {
		ID      uint32          `json:"id"`
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		return err
	}
	now := db.NowMS()
	status := "ok"
	if resp.Code != 0 {
		status = "error"
	}
	var dataStr *string
	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		s := string(resp.Data)
		dataStr = &s
	}
	_, err := p.DB.Exec(`UPDATE rpc_calls SET status=?, code=?, message=?, data_json=?, completed_at=? WHERE device_pk=? AND request_id=? AND status='pending'`,
		status, resp.Code, resp.Message, dataStr, now, d.ID, resp.ID)
	if err != nil {
		return err
	}
	call := models.RPCCall{DevicePK: d.ID, RequestID: resp.ID, Status: status, Code: &resp.Code, Message: resp.Message, DataJSON: dataStr, CompletedAt: &now}
	_ = p.DB.QueryRow(`SELECT id, method, params_json, correlation_id, created_at FROM rpc_calls WHERE device_pk=? AND request_id=? ORDER BY id DESC LIMIT 1`,
		d.ID, resp.ID).Scan(&call.ID, &call.Method, &call.ParamsJSON, &call.CorrelationID, &call.CreatedAt)
	key := fmt.Sprintf("%d:%d", d.ID, resp.ID)
	p.mu.RLock()
	ch := p.rpcWaiters[key]
	p.mu.RUnlock()
	if ch != nil {
		select {
		case ch <- call:
		default:
		}
	}
	p.emit("rpc.completed", call)
	p.publishRules(d.ProductID, d.DeviceID, "rpc/result", map[string]any{
		"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID,
		"request_id": resp.ID, "code": resp.Code, "message": resp.Message, "method": call.Method,
		"correlation_id": call.CorrelationID, "data": json.RawMessage(ptrStr(dataStr)),
	})
	return nil
}

func ptrStr(s *string) string {
	if s == nil {
		return "null"
	}
	return *s
}

func (p *Platform) ListRPC(devicePK int64, limit int) ([]models.RPCCall, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.DB.Query(`SELECT id, device_pk, request_id, method, params_json, status, code, message, data_json, correlation_id, created_at, completed_at
		FROM rpc_calls WHERE device_pk=? ORDER BY created_at DESC LIMIT ?`, devicePK, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.RPCCall
	for rows.Next() {
		var c models.RPCCall
		var code sql.NullInt64
		var data sql.NullString
		var completed sql.NullInt64
		if err := rows.Scan(&c.ID, &c.DevicePK, &c.RequestID, &c.Method, &c.ParamsJSON, &c.Status, &code, &c.Message, &data, &c.CorrelationID, &c.CreatedAt, &completed); err != nil {
			return nil, err
		}
		if code.Valid {
			v := int(code.Int64)
			c.Code = &v
		}
		if data.Valid {
			c.DataJSON = &data.String
		}
		c.CompletedAt = nullInt64(completed)
		out = append(out, c)
	}
	if out == nil {
		out = []models.RPCCall{}
	}
	return out, nil
}

func (p *Platform) HandleClientRPC(d *models.Device, payload []byte) error {
	var req struct {
		ID     uint32         `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return err
	}
	resp := map[string]any{"id": req.ID, "code": 501, "message": "not implemented", "data": nil}
	switch req.Method {
	case "issueUploadUrl":
		resp = map[string]any{
			"id": req.ID, "code": 0, "message": "ok",
			"data": map[string]any{"url": fmt.Sprintf("https://%s/upload/%s/%s", p.Cfg.BrokerHostPublic, d.ProductID, d.DeviceID), "expires_in": 3600},
		}
	}
	b, _ := json.Marshal(resp)
	if p.Pub == nil {
		return errors.New("mqtt not connected")
	}
	return p.Pub.Publish(p.DownTopic(d, "rpc", "response"), 1, false, b)
}

func (p *Platform) ReplyNTP(d *models.Device, payload []byte, recvMS int64) error {
	var req struct {
		ID             uint32 `json:"id"`
		DeviceSendTime int64  `json:"deviceSendTime"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return err
	}
	send := db.NowMS()
	resp := map[string]any{
		"id":             req.ID,
		"deviceSendTime": req.DeviceSendTime,
		"serverRecvTime": recvMS,
		"serverSendTime": send,
	}
	b, _ := json.Marshal(resp)
	if p.Pub == nil {
		return errors.New("mqtt not connected")
	}
	return p.Pub.Publish(p.DownTopic(d, "ntp", "response"), 1, false, b)
}
