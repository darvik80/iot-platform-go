package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"crearts-iot-platform/internal/db"
	"crearts-iot-platform/internal/models"
)

func (p *Platform) StoreEvent(d *models.Device, payload []byte) error {
	if p.SeenRecently(d.ID, "event", payload, 2*time.Second) {
		return nil
	}
	var e struct {
		TS       int64          `json:"ts"`
		ID       string         `json:"id"`
		Type     string         `json:"type"`
		Severity string         `json:"severity"`
		Code     string         `json:"code"`
		Message  string         `json:"message"`
		Data     map[string]any `json:"data"`
	}
	if err := json.Unmarshal(payload, &e); err != nil {
		return err
	}
	if err := p.RecordEvent(d, e.ID, e.Type, e.Severity, e.Code, e.Message, e.TS, e.Data); err != nil {
		return err
	}
	p.TouchSeen(d)
	return nil
}

// RecordEvent persists a discrete incident (device v1/e or platform-generated).
func (p *Platform) RecordEvent(d *models.Device, eventID, typ, severity, code, message string, ts int64, data map[string]any) error {
	if d == nil {
		return errors.New("device required")
	}
	if ts == 0 {
		ts = db.NowMS()
	}
	if severity == "" {
		severity = "info"
	}
	if typ == "" {
		typ = "info"
	}
	var dataJSON *string
	if data != nil {
		s := jsonValue(data)
		dataJSON = &s
	}
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO events(device_pk, event_id, type, severity, code, message, data_json, ts, ingested_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		d.ID, eventID, typ, severity, code, message, dataJSON, ts, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	ev := models.Event{ID: id, DevicePK: d.ID, ProductID: d.ProductID, DeviceID: d.DeviceID, EventID: eventID, Type: typ, Severity: severity, Code: code, Message: message, DataJSON: dataJSON, TS: ts, IngestedAt: now}
	p.emit("event", ev)
	p.publishRules(d.ProductID, d.DeviceID, "event", map[string]any{
		"ts": ts, "product_id": d.ProductID, "device_id": d.DeviceID,
		"type": typ, "severity": severity, "code": code, "message": message, "data": data,
	})
	return nil
}

func (p *Platform) ListEvents(devicePK int64, severity string, from, to int64, limit int) ([]models.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if to == 0 {
		to = db.NowMS()
	}
	q := `SELECT e.id, e.device_pk, pr.product_id, d.device_id, e.event_id, e.type, e.severity, e.code, e.message, e.data_json, e.ts, e.ingested_at
		FROM events e JOIN devices d ON d.id=e.device_pk JOIN products pr ON pr.id=d.product_pk WHERE e.ts>=? AND e.ts<=?`
	args := []any{from, to}
	if devicePK > 0 {
		q += ` AND e.device_pk=?`
		args = append(args, devicePK)
	}
	if severity != "" {
		q += ` AND e.severity=?`
		args = append(args, severity)
	}
	q += ` ORDER BY e.ts DESC LIMIT ?`
	args = append(args, limit)
	rows, err := p.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Event
	for rows.Next() {
		var e models.Event
		var data sql.NullString
		if err := rows.Scan(&e.ID, &e.DevicePK, &e.ProductID, &e.DeviceID, &e.EventID, &e.Type, &e.Severity, &e.Code, &e.Message, &data, &e.TS, &e.IngestedAt); err != nil {
			return nil, err
		}
		if data.Valid {
			e.DataJSON = &data.String
		}
		out = append(out, e)
	}
	if out == nil {
		out = []models.Event{}
	}
	return out, nil
}

func (p *Platform) StoreLogs(d *models.Device, payload []byte) error {
	var items []struct {
		TS      int64          `json:"ts"`
		Level   string         `json:"level"`
		Module  string         `json:"module"`
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Context map[string]any `json:"context"`
	}
	if err := json.Unmarshal(payload, &items); err != nil {
		var one struct {
			TS      int64          `json:"ts"`
			Level   string         `json:"level"`
			Module  string         `json:"module"`
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Context map[string]any `json:"context"`
		}
		if err2 := json.Unmarshal(payload, &one); err2 != nil {
			return err
		}
		items = append(items, one)
	}
	now := db.NowMS()
	for _, it := range items {
		if it.TS == 0 {
			it.TS = now
		}
		var ctx *string
		if it.Context != nil {
			s := jsonValue(it.Context)
			ctx = &s
		}
		_, _ = p.DB.Exec(`INSERT INTO device_logs(device_pk, level, module, code, message, context_json, ts, ingested_at) VALUES(?,?,?,?,?,?,?,?)`,
			d.ID, it.Level, it.Module, it.Code, it.Message, ctx, it.TS, now)
		p.emit("log", map[string]any{"device_id": d.ID, "level": it.Level, "module": it.Module, "message": it.Message, "ts": it.TS})
		p.publishRules(d.ProductID, d.DeviceID, "log", map[string]any{
			"ts": it.TS, "product_id": d.ProductID, "device_id": d.DeviceID,
			"level": it.Level, "module": it.Module, "code": it.Code, "message": it.Message, "context": it.Context,
		})
	}
	p.TouchSeen(d)
	return nil
}

func (p *Platform) ListLogs(devicePK int64, level string, from, to int64, limit int) ([]models.DeviceLog, error) {
	if limit <= 0 {
		limit = 200
	}
	if to == 0 {
		to = db.NowMS()
	}
	q := `SELECT l.id, l.device_pk, pr.product_id, d.device_id, l.level, l.module, l.code, l.message, l.context_json, l.ts, l.ingested_at
		FROM device_logs l JOIN devices d ON d.id=l.device_pk JOIN products pr ON pr.id=d.product_pk WHERE l.ts>=? AND l.ts<=?`
	args := []any{from, to}
	if devicePK > 0 {
		q += ` AND l.device_pk=?`
		args = append(args, devicePK)
	}
	if level != "" {
		q += ` AND l.level=?`
		args = append(args, level)
	}
	q += ` ORDER BY l.ts DESC LIMIT ?`
	args = append(args, limit)
	rows, err := p.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.DeviceLog
	for rows.Next() {
		var l models.DeviceLog
		var ctx sql.NullString
		if err := rows.Scan(&l.ID, &l.DevicePK, &l.ProductID, &l.DeviceID, &l.Level, &l.Module, &l.Code, &l.Message, &ctx, &l.TS, &l.IngestedAt); err != nil {
			return nil, err
		}
		if ctx.Valid {
			l.ContextJSON = &ctx.String
		}
		out = append(out, l)
	}
	if out == nil {
		out = []models.DeviceLog{}
	}
	return out, nil
}

func (p *Platform) HandleOTAVersion(d *models.Device, payload []byte) error {
	var v struct {
		Version string `json:"version"`
		Module  string `json:"module"`
	}
	if err := json.Unmarshal(payload, &v); err != nil {
		return err
	}
	if v.Module == "" {
		v.Module = "main"
	}
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET fw_version=?, updated_at=? WHERE id=?`, v.Version, now, d.ID)
	_ = p.MergeReported(d, map[string]any{"firmwareVersion": v.Version, "otaModule": v.Module})
	return nil
}

func (p *Platform) HandleOTAQuery(d *models.Device, payload []byte) error {
	var q struct {
		Module  string `json:"module"`
		Version string `json:"version"`
	}
	_ = json.Unmarshal(payload, &q)
	if q.Module == "" {
		q.Module = "main"
	}
	var fw models.OTAFirmware
	err := p.DB.QueryRow(`SELECT id, product_pk, module, version, size, sha256, sign_method, sign, url, created_at
		FROM ota_firmwares WHERE product_pk=? AND module=? ORDER BY created_at DESC LIMIT 1`, d.ProductPK, q.Module).
		Scan(&fw.ID, &fw.ProductPK, &fw.Module, &fw.Version, &fw.Size, &fw.SHA256, &fw.SignMethod, &fw.Sign, &fw.URL, &fw.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if fw.Version == q.Version {
		return nil
	}
	return p.PushOTA(d, &fw, false)
}

func (p *Platform) HandleOTAProgress(d *models.Device, payload []byte) error {
	var prog struct {
		Module string `json:"module"`
		Step   int    `json:"step"`
		Desc   string `json:"desc"`
	}
	if err := json.Unmarshal(payload, &prog); err != nil {
		return err
	}
	status := "running"
	switch {
	case prog.Step < 0:
		status = "failed"
	case prog.Step >= 101:
		status = "done"
	}
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE ota_jobs SET status=?, step=?, desc=?, updated_at=? WHERE device_pk=? AND status IN ('pending','running')`,
		status, prog.Step, prog.Desc, now, d.ID)
	p.emit("ota.progress", map[string]any{"device_id": d.ID, "step": prog.Step, "desc": prog.Desc, "status": status})
	p.publishRules(d.ProductID, d.DeviceID, "ota", map[string]any{
		"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "step": prog.Step, "desc": prog.Desc, "status": status,
	})
	return nil
}

type FirmwareInput struct {
	ProductID  string `json:"product_id"`
	Module     string `json:"module"`
	Version    string `json:"version"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	SignMethod string `json:"sign_method"`
	Sign       string `json:"sign"`
	URL        string `json:"url"`
}

func (p *Platform) RegisterFirmware(in FirmwareInput) (*models.OTAFirmware, error) {
	pr, err := p.GetProductBySlug(in.ProductID)
	if err != nil {
		return nil, err
	}
	if in.Module == "" {
		in.Module = "main"
	}
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO ota_firmwares(product_pk, module, version, size, sha256, sign_method, sign, url, created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		pr.ID, in.Module, in.Version, in.Size, in.SHA256, in.SignMethod, in.Sign, in.URL, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.OTAFirmware{ID: id, ProductPK: pr.ID, ProductID: pr.ProductID, Module: in.Module, Version: in.Version, Size: in.Size, SHA256: in.SHA256, SignMethod: in.SignMethod, Sign: in.Sign, URL: in.URL, CreatedAt: now}, nil
}

func (p *Platform) ListFirmware(productID string) ([]models.OTAFirmware, error) {
	q := `SELECT f.id, f.product_pk, pr.product_id, f.module, f.version, f.size, f.sha256, f.sign_method, f.sign, f.url, f.created_at
		FROM ota_firmwares f JOIN products pr ON pr.id=f.product_pk`
	args := []any{}
	if productID != "" {
		q += ` WHERE pr.product_id=?`
		args = append(args, productID)
	}
	q += ` ORDER BY f.created_at DESC`
	rows, err := p.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.OTAFirmware
	for rows.Next() {
		var f models.OTAFirmware
		if err := rows.Scan(&f.ID, &f.ProductPK, &f.ProductID, &f.Module, &f.Version, &f.Size, &f.SHA256, &f.SignMethod, &f.Sign, &f.URL, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if out == nil {
		out = []models.OTAFirmware{}
	}
	return out, nil
}

func (p *Platform) StartOTAJob(devicePK, firmwareID int64) (*models.OTAJob, error) {
	d, err := p.GetDevice(devicePK)
	if err != nil {
		return nil, err
	}
	var fw models.OTAFirmware
	err = p.DB.QueryRow(`SELECT id, product_pk, module, version, size, sha256, sign_method, sign, url, created_at FROM ota_firmwares WHERE id=?`, firmwareID).
		Scan(&fw.ID, &fw.ProductPK, &fw.Module, &fw.Version, &fw.Size, &fw.SHA256, &fw.SignMethod, &fw.Sign, &fw.URL, &fw.CreatedAt)
	if err != nil {
		return nil, errors.New("firmware not found")
	}
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO ota_jobs(device_pk, firmware_id, status, step, desc, created_at, updated_at) VALUES(?,?, 'pending', 0, 'queued', ?, ?)`,
		devicePK, firmwareID, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := p.PushOTA(d, &fw, false); err != nil {
		return nil, err
	}
	_, _ = p.DB.Exec(`UPDATE ota_jobs SET status='running', updated_at=? WHERE id=?`, db.NowMS(), id)
	return &models.OTAJob{ID: id, DevicePK: devicePK, FirmwareID: firmwareID, Status: "running", Step: 0, Desc: "notified", CreatedAt: now, UpdatedAt: now}, nil
}

func (p *Platform) PushOTA(d *models.Device, fw *models.OTAFirmware, force bool) error {
	body := map[string]any{
		"version": fw.Version, "module": fw.Module, "size": fw.Size, "url": fw.URL,
		"sha256": fw.SHA256, "signMethod": fw.SignMethod, "sign": fw.Sign, "force": force,
	}
	b, _ := json.Marshal(body)
	if p.Pub == nil {
		return errors.New("mqtt not connected")
	}
	return p.Pub.Publish(p.DownTopic(d, "ota", "update"), 1, false, b)
}

func (p *Platform) CancelOTA(d *models.Device, module, reason string) error {
	if module == "" {
		module = "main"
	}
	body := map[string]any{"module": module, "reason": reason}
	b, _ := json.Marshal(body)
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE ota_jobs SET status='cancelled', step=-4, desc=?, updated_at=? WHERE device_pk=? AND status IN ('pending','running')`,
		reason, now, d.ID)
	if p.Pub == nil {
		return errors.New("mqtt not connected")
	}
	return p.Pub.Publish(p.DownTopic(d, "ota", "cancel"), 1, false, b)
}

func (p *Platform) ListOTAJobs(devicePK int64) ([]models.OTAJob, error) {
	q := `SELECT id, device_pk, firmware_id, status, step, desc, created_at, updated_at FROM ota_jobs`
	args := []any{}
	if devicePK > 0 {
		q += ` WHERE device_pk=?`
		args = append(args, devicePK)
	}
	q += ` ORDER BY created_at DESC LIMIT 100`
	rows, err := p.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.OTAJob
	for rows.Next() {
		var j models.OTAJob
		if err := rows.Scan(&j.ID, &j.DevicePK, &j.FirmwareID, &j.Status, &j.Step, &j.Desc, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	if out == nil {
		out = []models.OTAJob{}
	}
	return out, nil
}
