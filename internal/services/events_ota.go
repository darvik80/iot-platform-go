package services

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
)

func (p *Platform) StoreEvent(d *models.Device, payload []byte) error {
	if p.SeenRecently(d.AccountID, d.ID, protocol.BusEvent, payload, 2*time.Second) {
		if p.Log != nil {
			p.Log.Info("event dedup",
				"product", d.ProductID, "device", d.DeviceID,
				"bytes", len(payload))
		}
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
	res, err := p.DB.Exec(`INSERT INTO events(device_pk, account_id, event_id, type, severity, code, message, data_json, ts, ingested_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.AccountID, eventID, typ, severity, code, message, dataJSON, ts, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	ev := models.Event{ID: id, DevicePK: d.ID, ProductID: d.ProductID, DeviceID: d.DeviceID, EventID: eventID, Type: typ, Severity: severity, Code: code, Message: message, DataJSON: dataJSON, TS: ts, IngestedAt: now}
	p.emit(d.AccountID, "event", ev)
	p.publishRules(d.AccountSlug, d.ProductID, d.DeviceID, protocol.BusEvent, map[string]any{
		"ts": ts, "product_id": d.ProductID, "device_id": d.DeviceID,
		"type": typ, "severity": severity, "code": code, "message": message, "data": data,
	})
	return nil
}

func (p *Platform) ListEvents(accountID int64, devicePK int64, severity string, from, to int64, limit int) ([]models.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if to == 0 {
		to = db.NowMS()
	}
	q := `SELECT e.id, e.device_pk, pr.product_id, d.device_id, e.event_id, e.type, e.severity, e.code, e.message, e.data_json, e.ts, e.ingested_at
		FROM events e JOIN devices d ON d.id=e.device_pk JOIN products pr ON pr.id=d.product_pk WHERE e.ts>=? AND e.ts<=?`
	args := []any{from, to}
	if accountID > 0 {
		q += ` AND pr.account_id=?`
		args = append(args, accountID)
	}
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
		_, _ = p.DB.Exec(`INSERT INTO device_logs(device_pk, account_id, level, module, code, message, context_json, ts, ingested_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			d.ID, d.AccountID, it.Level, it.Module, it.Code, it.Message, ctx, it.TS, now)
		p.emit(d.AccountID, "log", map[string]any{"device_id": d.ID, "level": it.Level, "module": it.Module, "message": it.Message, "ts": it.TS})
		p.publishRules(d.AccountSlug, d.ProductID, d.DeviceID, protocol.BusLog, map[string]any{
			"ts": it.TS, "product_id": d.ProductID, "device_id": d.DeviceID,
			"level": it.Level, "module": it.Module, "code": it.Code, "message": it.Message, "context": it.Context,
		})
	}
	p.TouchSeen(d)
	return nil
}

func (p *Platform) ListLogs(accountID int64, devicePK int64, level string, from, to int64, limit int) ([]models.DeviceLog, error) {
	if limit <= 0 {
		limit = 200
	}
	if to == 0 {
		to = db.NowMS()
	}
	q := `SELECT l.id, l.device_pk, pr.product_id, d.device_id, l.level, l.module, l.code, l.message, l.context_json, l.ts, l.ingested_at
		FROM device_logs l JOIN devices d ON d.id=l.device_pk JOIN products pr ON pr.id=d.product_pk WHERE l.ts>=? AND l.ts<=?`
	args := []any{from, to}
	if accountID > 0 {
		q += ` AND pr.account_id=?`
		args = append(args, accountID)
	}
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
	d.FwVersion = v.Version
	_ = p.MergeReported(d, map[string]any{"firmwareVersion": v.Version, "otaModule": v.Module})
	return p.maybeAutoOTA(d, v.Version, v.Module)
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
	fw, err := p.latestFirmware(d.ProductPK, q.Module)
	if err != nil || fw == nil {
		return err
	}
	if fw.Version == q.Version {
		return nil
	}
	_, err = p.StartOTAJob(d.ID, fw.ID, false)
	return err
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
	_, _ = p.DB.Exec(`UPDATE ota_jobs SET status=?, step=?, description=?, updated_at=? WHERE device_pk=? AND status IN ('pending','running')`,
		status, prog.Step, prog.Desc, now, d.ID)
	p.emit(d.AccountID, "ota.progress", map[string]any{"device_id": d.ID, "step": prog.Step, "description": prog.Desc, "status": status})
	p.publishRules(d.AccountSlug, d.ProductID, d.DeviceID, protocol.BusOTA, map[string]any{
		"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "step": prog.Step, "description": prog.Desc, "status": status,
	})
	return nil
}

const MaxFirmwareBytes = 32 << 20

var downloadTokenRe = regexp.MustCompile(`^[a-f0-9]{32}$`)

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

func newDownloadToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (p *Platform) hydrateFirmware(fw *models.OTAFirmware) {
	if fw == nil {
		return
	}
	fw.DownloadToken = strings.TrimSpace(fw.DownloadToken)
	if fw.DownloadToken != "" {
		fw.Hosted = true
		fw.URL = p.Cfg.FirmwarePublicURL(fw.DownloadToken)
	}
}

func uniqueFirmwareErr(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique") {
		return errors.New("firmware version already exists for this product/module")
	}
	return err
}

func (p *Platform) firmwareByVersion(productPK int64, module, version string) (*models.OTAFirmware, error) {
	if module == "" {
		module = "main"
	}
	var fw models.OTAFirmware
	err := p.DB.QueryRow(`SELECT id, product_pk, module, version, size, sha256, sign_method, sign, url, COALESCE(download_token,''), created_at
		FROM ota_firmwares WHERE product_pk=? AND module=? AND version=?`, productPK, module, version).
		Scan(&fw.ID, &fw.ProductPK, &fw.Module, &fw.Version, &fw.Size, &fw.SHA256, &fw.SignMethod, &fw.Sign, &fw.URL, &fw.DownloadToken, &fw.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.hydrateFirmware(&fw)
	return &fw, nil
}

func (p *Platform) removeHostedFile(token string) {
	token = strings.TrimSpace(token)
	if !downloadTokenRe.MatchString(token) {
		return
	}
	_ = os.Remove(filepath.Join(p.Cfg.FirmwareDir(), token+".bin"))
}

func (p *Platform) RegisterFirmware(accountID int64, in FirmwareInput) (*models.OTAFirmware, error) {
	pr, err := p.GetProductBySlugInAccount(accountID, in.ProductID)
	if err != nil {
		return nil, err
	}
	in.Version = strings.TrimSpace(in.Version)
	if in.Version == "" {
		return nil, errors.New("version required")
	}
	in.URL = strings.TrimSpace(in.URL)
	if in.URL == "" {
		return nil, errors.New("url required (or upload a file)")
	}
	if in.Module == "" {
		in.Module = "main"
	}
	now := db.NowMS()
	if existing, err := p.firmwareByVersion(pr.ID, in.Module, in.Version); err != nil {
		return nil, err
	} else if existing != nil {
		oldToken := existing.DownloadToken
		if _, err := p.DB.Exec(`UPDATE ota_firmwares SET size=?, sha256=?, sign_method=?, sign=?, url=?, download_token='', created_at=? WHERE id=?`,
			in.Size, in.SHA256, in.SignMethod, in.Sign, in.URL, now, existing.ID); err != nil {
			return nil, uniqueFirmwareErr(err)
		}
		p.removeHostedFile(oldToken)
		fw := *existing
		fw.Size, fw.SHA256, fw.SignMethod, fw.Sign, fw.URL = in.Size, in.SHA256, in.SignMethod, in.Sign, in.URL
		fw.DownloadToken, fw.Hosted, fw.CreatedAt = "", false, now
		if pr.AutoOTA {
			_, _ = p.NotifyFirmware(fw.ID, 0, false)
		}
		return &fw, nil
	}
	res, err := p.DB.Exec(`INSERT INTO ota_firmwares(product_pk, account_id, module, version, size, sha256, sign_method, sign, url, created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		pr.ID, pr.AccountID, in.Module, in.Version, in.Size, in.SHA256, in.SignMethod, in.Sign, in.URL, now)
	if err != nil {
		return nil, uniqueFirmwareErr(err)
	}
	id, _ := res.LastInsertId()
	fw := &models.OTAFirmware{ID: id, ProductPK: pr.ID, ProductID: pr.ProductID, Module: in.Module, Version: in.Version, Size: in.Size, SHA256: in.SHA256, SignMethod: in.SignMethod, Sign: in.Sign, URL: in.URL, CreatedAt: now}
	if pr.AutoOTA {
		_, _ = p.NotifyFirmware(fw.ID, 0, false)
	}
	return fw, nil
}

func (p *Platform) StoreFirmwareFile(accountID int64, in FirmwareInput, r io.Reader) (*models.OTAFirmware, error) {
	pr, err := p.GetProductBySlugInAccount(accountID, in.ProductID)
	if err != nil {
		return nil, err
	}
	in.Version = strings.TrimSpace(in.Version)
	if in.Version == "" {
		return nil, errors.New("version required")
	}
	if in.Module == "" {
		in.Module = "main"
	}
	if strings.TrimSpace(in.SignMethod) == "" {
		in.SignMethod = "sha256"
	}
	token, err := newDownloadToken()
	if err != nil {
		return nil, err
	}
	dir := p.Cfg.FirmwareDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(dir, token+".bin")
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, MaxFirmwareBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return nil, closeErr
	}
	if n == 0 {
		_ = os.Remove(dest)
		return nil, errors.New("empty firmware file")
	}
	if n > MaxFirmwareBytes {
		_ = os.Remove(dest)
		return nil, errors.New("firmware larger than 32MB")
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if in.SHA256 != "" && !strings.EqualFold(strings.TrimSpace(in.SHA256), sum) {
		_ = os.Remove(dest)
		return nil, errors.New("sha256 mismatch")
	}
	if in.Size > 0 && in.Size != n {
		_ = os.Remove(dest)
		return nil, errors.New("size mismatch")
	}
	url := p.Cfg.FirmwarePublicURL(token)
	now := db.NowMS()
	existing, err := p.firmwareByVersion(pr.ID, in.Module, in.Version)
	if err != nil {
		_ = os.Remove(dest)
		return nil, err
	}
	if existing != nil {
		oldToken := existing.DownloadToken
		if _, err := p.DB.Exec(`UPDATE ota_firmwares SET size=?, sha256=?, sign_method=?, sign=?, url=?, download_token=?, created_at=? WHERE id=?`,
			n, sum, in.SignMethod, in.Sign, url, token, now, existing.ID); err != nil {
			_ = os.Remove(dest)
			return nil, uniqueFirmwareErr(err)
		}
		if oldToken != "" && oldToken != token {
			p.removeHostedFile(oldToken)
		}
		fw := *existing
		fw.ProductID = pr.ProductID
		fw.Size, fw.SHA256, fw.SignMethod, fw.Sign = n, sum, in.SignMethod, in.Sign
		fw.URL, fw.DownloadToken, fw.Hosted, fw.CreatedAt = url, token, true, now
		if pr.AutoOTA {
			_, _ = p.NotifyFirmware(fw.ID, 0, false)
		}
		return &fw, nil
	}
	res, err := p.DB.Exec(`INSERT INTO ota_firmwares(product_pk, account_id, module, version, size, sha256, sign_method, sign, url, download_token, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		pr.ID, pr.AccountID, in.Module, in.Version, n, sum, in.SignMethod, in.Sign, url, token, now)
	if err != nil {
		_ = os.Remove(dest)
		return nil, uniqueFirmwareErr(err)
	}
	id, _ := res.LastInsertId()
	fw := &models.OTAFirmware{
		ID: id, ProductPK: pr.ID, ProductID: pr.ProductID, Module: in.Module, Version: in.Version,
		Size: n, SHA256: sum, SignMethod: in.SignMethod, Sign: in.Sign, URL: url,
		DownloadToken: token, Hosted: true, CreatedAt: now,
	}
	if pr.AutoOTA {
		_, _ = p.NotifyFirmware(fw.ID, 0, false)
	}
	return fw, nil
}

func (p *Platform) HostedFirmware(token string) (*models.OTAFirmware, string, error) {
	token = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(token)), ".bin")
	if !downloadTokenRe.MatchString(token) {
		return nil, "", errors.New("firmware not found")
	}
	var fw models.OTAFirmware
	err := p.DB.QueryRow(`SELECT f.id, f.product_pk, pr.product_id, f.module, f.version, f.size, f.sha256, f.sign_method, f.sign, f.url, COALESCE(f.download_token,''), f.created_at
		FROM ota_firmwares f JOIN products pr ON pr.id=f.product_pk WHERE f.download_token=?`, token).
		Scan(&fw.ID, &fw.ProductPK, &fw.ProductID, &fw.Module, &fw.Version, &fw.Size, &fw.SHA256, &fw.SignMethod, &fw.Sign, &fw.URL, &fw.DownloadToken, &fw.CreatedAt)
	if err != nil {
		return nil, "", errors.New("firmware not found")
	}
	p.hydrateFirmware(&fw)
	path := filepath.Join(p.Cfg.FirmwareDir(), token+".bin")
	if _, err := os.Stat(path); err != nil {
		return nil, "", errors.New("firmware file missing")
	}
	return &fw, path, nil
}

func (p *Platform) ListFirmware(accountID int64, productID string) ([]models.OTAFirmware, error) {
	q := `SELECT f.id, f.product_pk, pr.product_id, f.module, f.version, f.size, f.sha256, f.sign_method, f.sign, f.url, COALESCE(f.download_token,''), f.created_at
		FROM ota_firmwares f JOIN products pr ON pr.id=f.product_pk WHERE 1=1`
	args := []any{}
	if accountID > 0 {
		q += ` AND pr.account_id=?`
		args = append(args, accountID)
	}
	if productID != "" {
		q += ` AND pr.product_id=?`
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
		if err := rows.Scan(&f.ID, &f.ProductPK, &f.ProductID, &f.Module, &f.Version, &f.Size, &f.SHA256, &f.SignMethod, &f.Sign, &f.URL, &f.DownloadToken, &f.CreatedAt); err != nil {
			return nil, err
		}
		p.hydrateFirmware(&f)
		out = append(out, f)
	}
	if out == nil {
		out = []models.OTAFirmware{}
	}
	return out, nil
}

func (p *Platform) latestFirmware(productPK int64, module string) (*models.OTAFirmware, error) {
	if module == "" {
		module = "main"
	}
	var fw models.OTAFirmware
	err := p.DB.QueryRow(`SELECT id, product_pk, module, version, size, sha256, sign_method, sign, url, COALESCE(download_token,''), created_at
		FROM ota_firmwares WHERE product_pk=? AND module=? ORDER BY created_at DESC LIMIT 1`, productPK, module).
		Scan(&fw.ID, &fw.ProductPK, &fw.Module, &fw.Version, &fw.Size, &fw.SHA256, &fw.SignMethod, &fw.Sign, &fw.URL, &fw.DownloadToken, &fw.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.hydrateFirmware(&fw)
	return &fw, nil
}

func (p *Platform) firmwareByID(id int64) (*models.OTAFirmware, error) {
	var fw models.OTAFirmware
	err := p.DB.QueryRow(`SELECT f.id, f.product_pk, pr.product_id, f.module, f.version, f.size, f.sha256, f.sign_method, f.sign, f.url, COALESCE(f.download_token,''), f.created_at
		FROM ota_firmwares f JOIN products pr ON pr.id=f.product_pk WHERE f.id=?`, id).
		Scan(&fw.ID, &fw.ProductPK, &fw.ProductID, &fw.Module, &fw.Version, &fw.Size, &fw.SHA256, &fw.SignMethod, &fw.Sign, &fw.URL, &fw.DownloadToken, &fw.CreatedAt)
	if err != nil {
		return nil, errors.New("firmware not found")
	}
	p.hydrateFirmware(&fw)
	return &fw, nil
}

// FirmwareInAccount reports whether the firmware belongs to a product of accountID.
func (p *Platform) FirmwareInAccount(accountID, id int64) bool {
	var acc int64
	err := p.DB.QueryRow(`SELECT COALESCE(pr.account_id,0) FROM ota_firmwares f JOIN products pr ON pr.id=f.product_pk WHERE f.id=?`, id).Scan(&acc)
	return err == nil && acc == accountID
}

func (p *Platform) DeleteFirmware(id int64) error {
	fw, err := p.firmwareByID(id)
	if err != nil {
		return err
	}
	if _, err := p.DB.Exec(`DELETE FROM ota_jobs WHERE firmware_id=?`, id); err != nil {
		return err
	}
	res, err := p.DB.Exec(`DELETE FROM ota_firmwares WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("firmware not found")
	}
	p.removeHostedFile(fw.DownloadToken)
	return nil
}

func (p *Platform) otaAutoEnabled(d *models.Device) bool {
	if d == nil || !d.Enabled || !d.AutoOTA {
		return false
	}
	pr, err := p.GetProduct(d.ProductPK)
	return err == nil && pr.AutoOTA
}

func (p *Platform) maybeAutoOTA(d *models.Device, currentVersion, module string) error {
	if !p.otaAutoEnabled(d) {
		return nil
	}
	fw, err := p.latestFirmware(d.ProductPK, module)
	if err != nil || fw == nil {
		return err
	}
	if currentVersion != "" && fw.Version == currentVersion {
		return nil
	}
	_, err = p.StartOTAJob(d.ID, fw.ID, false)
	return err
}

func (p *Platform) flushPendingOTA(d *models.Device) {
	if d == nil || d.Status != "online" {
		return
	}
	rows, err := p.DB.Query(`SELECT id, firmware_id FROM ota_jobs WHERE device_pk=? AND status='pending' ORDER BY id`, d.ID)
	if err != nil {
		return
	}
	defer rows.Close()
	type pending struct {
		id, fwID int64
	}
	var list []pending
	for rows.Next() {
		var it pending
		if rows.Scan(&it.id, &it.fwID) == nil {
			list = append(list, it)
		}
	}
	for _, it := range list {
		fw, err := p.firmwareByID(it.fwID)
		if err != nil {
			continue
		}
		if err := p.PushOTA(d, fw, false); err != nil {
			continue
		}
		_, _ = p.DB.Exec(`UPDATE ota_jobs SET status='running', description='notified', updated_at=? WHERE id=?`, db.NowMS(), it.id)
	}
}

func (p *Platform) StartOTAJob(devicePK, firmwareID int64, force bool) (*models.OTAJob, error) {
	d, err := p.GetDevice(devicePK)
	if err != nil {
		return nil, err
	}
	fw, err := p.firmwareByID(firmwareID)
	if err != nil {
		return nil, err
	}
	if fw.ProductPK != d.ProductPK {
		return nil, errors.New("firmware does not belong to this product")
	}
	if !force && d.FwVersion != "" && d.FwVersion == fw.Version {
		return nil, errors.New("already on this version")
	}
	var existing int64
	_ = p.DB.QueryRow(`SELECT id FROM ota_jobs WHERE device_pk=? AND firmware_id=? AND status IN ('pending','running') ORDER BY id DESC LIMIT 1`,
		devicePK, firmwareID).Scan(&existing)
	now := db.NowMS()
	if existing > 0 {
		job := &models.OTAJob{ID: existing, DevicePK: devicePK, ProductID: d.ProductID, DeviceID: d.DeviceID, FirmwareID: firmwareID, FirmwareVersion: fw.Version, Module: fw.Module, Status: "pending", CreatedAt: now, UpdatedAt: now}
		if d.Status == "online" {
			if err := p.PushOTA(d, fw, force); err != nil {
				return nil, err
			}
			_, _ = p.DB.Exec(`UPDATE ota_jobs SET status='running', description='notified', updated_at=? WHERE id=?`, now, existing)
			job.Status = "running"
			job.Description = "notified"
		}
		return job, nil
	}
	res, err := p.DB.Exec(`INSERT INTO ota_jobs(device_pk, account_id, firmware_id, status, step, description, created_at, updated_at) VALUES(?,?,?, 'pending', 0, 'queued', ?, ?)`,
		devicePK, d.AccountID, firmwareID, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	job := &models.OTAJob{ID: id, DevicePK: devicePK, ProductID: d.ProductID, DeviceID: d.DeviceID, FirmwareID: firmwareID, FirmwareVersion: fw.Version, Module: fw.Module, Status: "pending", Step: 0, Description: "queued", CreatedAt: now, UpdatedAt: now}
	if d.Status != "online" {
		return job, nil
	}
	if err := p.PushOTA(d, fw, force); err != nil {
		return nil, err
	}
	_, _ = p.DB.Exec(`UPDATE ota_jobs SET status='running', description='notified', updated_at=? WHERE id=?`, db.NowMS(), id)
	job.Status = "running"
	job.Description = "notified"
	return job, nil
}

func (p *Platform) NotifyFirmware(firmwareID, devicePK int64, force bool) (*models.OTANotifyResult, error) {
	fw, err := p.firmwareByID(firmwareID)
	if err != nil {
		return nil, err
	}
	out := &models.OTANotifyResult{FirmwareID: firmwareID, Jobs: []models.OTAJob{}}
	targets := []models.Device{}
	if devicePK > 0 {
		d, err := p.GetDevice(devicePK)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *d)
	} else {
		pr, err := p.GetProduct(fw.ProductPK)
		if err != nil {
			return nil, err
		}
		list, err := p.ListDevices(pr.AccountID, fw.ProductID, "", "")
		if err != nil {
			return nil, err
		}
		for _, d := range list {
			if d.ProductPK == fw.ProductPK {
				targets = append(targets, d)
			}
		}
	}
	for _, d := range targets {
		if !d.Enabled {
			out.Skipped++
			continue
		}
		job, err := p.StartOTAJob(d.ID, firmwareID, force)
		if err != nil {
			if strings.Contains(err.Error(), "already on this version") {
				out.Skipped++
				continue
			}
			out.Failed++
			out.Errors = append(out.Errors, d.ProductID+"."+d.DeviceID+": "+err.Error())
			continue
		}
		out.Started++
		out.Jobs = append(out.Jobs, *job)
	}
	return out, nil
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
	return p.publishDown(d, protocol.CapOTA, protocol.OpUpdate, b)
}

func (p *Platform) CancelOTA(d *models.Device, module, reason string) error {
	if module == "" {
		module = "main"
	}
	body := map[string]any{"module": module, "reason": reason}
	b, _ := json.Marshal(body)
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE ota_jobs SET status='cancelled', step=-4, description=?, updated_at=? WHERE device_pk=? AND status IN ('pending','running')`,
		reason, now, d.ID)
	if p.Pub == nil {
		return errors.New("mqtt not connected")
	}
	return p.publishDown(d, protocol.CapOTA, protocol.OpCancel, b)
}

func (p *Platform) ListOTAJobs(accountID int64, devicePK int64) ([]models.OTAJob, error) {
	q := `SELECT j.id, j.device_pk, pr.product_id, d.device_id, j.firmware_id, f.version, f.module, j.status, j.step, j.description, j.created_at, j.updated_at
		FROM ota_jobs j
		JOIN devices d ON d.id=j.device_pk
		JOIN products pr ON pr.id=d.product_pk
		JOIN ota_firmwares f ON f.id=j.firmware_id WHERE 1=1`
	args := []any{}
	if accountID > 0 {
		q += ` AND pr.account_id=?`
		args = append(args, accountID)
	}
	if devicePK > 0 {
		q += ` AND j.device_pk=?`
		args = append(args, devicePK)
	}
	q += ` ORDER BY j.created_at DESC LIMIT 100`
	rows, err := p.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.OTAJob
	for rows.Next() {
		var j models.OTAJob
		if err := rows.Scan(&j.ID, &j.DevicePK, &j.ProductID, &j.DeviceID, &j.FirmwareID, &j.FirmwareVersion, &j.Module, &j.Status, &j.Step, &j.Description, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	if out == nil {
		out = []models.OTAJob{}
	}
	return out, nil
}
