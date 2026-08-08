package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"crearts-iot-platform/internal/auth"
	"crearts-iot-platform/internal/broker"
	"crearts-iot-platform/internal/db"
	"crearts-iot-platform/internal/models"
)

func slugifyID(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case r == '.' || r == '_' || r == '-':
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		case unicode.IsSpace(r):
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-._")
	if len(out) > 63 {
		out = out[:63]
	}
	return out
}

type DeviceInput struct {
	ProductID   string   `json:"product_id"`
	DeviceID    string   `json:"device_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	TopicStyle  string   `json:"topic_style"`
}

func (p *Platform) ListDevices(productID, status, q string) ([]models.Device, error) {
	query := `
		SELECT d.id, d.product_pk, pr.product_id, d.device_id, d.name, d.description, d.tags,
		       d.access_token_hash, d.token_prefix, d.status, d.last_seen_at, d.last_ip, d.fw_version,
		       d.topic_style, d.enabled, d.created_at, d.updated_at
		FROM devices d JOIN products pr ON pr.id = d.product_pk WHERE 1=1`
	args := []any{}
	if productID != "" {
		query += ` AND pr.product_id=?`
		args = append(args, productID)
	}
	if status != "" {
		query += ` AND d.status=?`
		args = append(args, status)
	}
	if q != "" {
		query += ` AND (d.device_id LIKE ? OR d.name LIKE ? OR pr.product_id LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	query += ` ORDER BY d.updated_at DESC`
	rows, err := p.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	if out == nil {
		out = []models.Device{}
	}
	return out, nil
}

func (p *Platform) GetDevice(id int64) (*models.Device, error) {
	row := p.DB.QueryRow(`
		SELECT d.id, d.product_pk, pr.product_id, d.device_id, d.name, d.description, d.tags,
		       d.access_token_hash, d.token_prefix, d.status, d.last_seen_at, d.last_ip, d.fw_version,
		       d.topic_style, d.enabled, d.created_at, d.updated_at
		FROM devices d JOIN products pr ON pr.id = d.product_pk WHERE d.id=?`, id)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("not found")
	}
	return d, err
}

func (p *Platform) GetDeviceByIdentity(productID, deviceID string) (*models.Device, error) {
	row := p.DB.QueryRow(`
		SELECT d.id, d.product_pk, pr.product_id, d.device_id, d.name, d.description, d.tags,
		       d.access_token_hash, d.token_prefix, d.status, d.last_seen_at, d.last_ip, d.fw_version,
		       d.topic_style, d.enabled, d.created_at, d.updated_at
		FROM devices d JOIN products pr ON pr.id = d.product_pk
		WHERE pr.product_id=? AND d.device_id=?`, productID, deviceID)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("not found")
	}
	return d, err
}

func (p *Platform) CreateDevice(in DeviceInput) (*models.Device, *models.DeviceCredentials, error) {
	in.ProductID = strings.TrimSpace(in.ProductID)
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.Name = strings.TrimSpace(in.Name)
	if err := ValidateIDs(in.ProductID, in.DeviceID); err != nil {
		return nil, nil, err
	}
	pr, err := p.GetProductBySlug(in.ProductID)
	if err != nil {
		return nil, nil, fmt.Errorf("product: %w", err)
	}
	style := in.TopicStyle
	if style == "" {
		style = pr.TopicStyleDefault
	}
	if style != "short" && style != "full" {
		return nil, nil, errors.New("invalid topic_style")
	}
	if in.Name == "" {
		in.Name = in.DeviceID
	}
	token, err := auth.GenerateAccessToken()
	if err != nil {
		return nil, nil, err
	}
	hash := auth.HashToken(token)
	prefix := auth.TokenPrefix(token)

	// Provision broker user BEFORE inserting the device so we never hand out a dead token.
	if p.Broker == nil {
		return nil, nil, errors.New("broker manager not configured")
	}
	if err := p.Broker.Ping(); err != nil {
		return nil, nil, fmt.Errorf("rabbitmq management: %w", err)
	}
	if err := p.Broker.UpsertDeviceUser(token, in.ProductID, in.DeviceID); err != nil {
		return nil, nil, fmt.Errorf("rabbitmq provision failed: %w", err)
	}

	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO devices(product_pk, device_id, name, description, tags, access_token_hash, token_prefix, status, topic_style, enabled, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,1,?,?)`,
		pr.ID, in.DeviceID, in.Name, in.Description, tagsJSON(in.Tags), hash, prefix, "offline", style, now, now)
	if err != nil {
		_ = p.Broker.DeleteUser(broker.DeviceMQTTUsername(in.ProductID, in.DeviceID))
		return nil, nil, err
	}
	id, _ := res.LastInsertId()
	_, _ = p.DB.Exec(`INSERT INTO device_credentials(device_pk, token_hash, token_prefix, created_at) VALUES(?,?,?,?)`,
		id, hash, prefix, now)
	_ = p.ReloadDeviceMaps()
	d, err := p.GetDevice(id)
	if err != nil {
		return nil, nil, err
	}
	creds := deviceCreds(in.ProductID, in.DeviceID, token, p.Cfg.BrokerHostPublic, style)
	p.Log.Info("device created with rabbitmq user", "product", in.ProductID, "device", in.DeviceID, "mqtt_user", creds.Username, "token_prefix", prefix)
	return d, creds, nil
}

func (p *Platform) RotateDeviceToken(id int64) (*models.DeviceCredentials, error) {
	d, err := p.GetDevice(id)
	if err != nil {
		return nil, err
	}
	token, err := auth.GenerateAccessToken()
	if err != nil {
		return nil, err
	}
	if p.Broker == nil {
		return nil, errors.New("broker manager not configured")
	}
	if err := p.Broker.Ping(); err != nil {
		return nil, fmt.Errorf("rabbitmq management: %w", err)
	}
	if err := p.Broker.UpsertDeviceUser(token, d.ProductID, d.DeviceID); err != nil {
		return nil, fmt.Errorf("rabbitmq provision failed: %w", err)
	}
	hash := auth.HashToken(token)
	prefix := auth.TokenPrefix(token)
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE device_credentials SET revoked_at=? WHERE device_pk=? AND revoked_at IS NULL`, now, id)
	_, err = p.DB.Exec(`UPDATE devices SET access_token_hash=?, token_prefix=?, updated_at=? WHERE id=?`, hash, prefix, now, id)
	if err != nil {
		return nil, err
	}
	_, _ = p.DB.Exec(`INSERT INTO device_credentials(device_pk, token_hash, token_prefix, created_at) VALUES(?,?,?,?)`, id, hash, prefix, now)
	_ = p.ReloadDeviceMaps()
	p.Log.Info("device token rotated with rabbitmq user", "product", d.ProductID, "device", d.DeviceID, "mqtt_user", broker.DeviceMQTTUsername(d.ProductID, d.DeviceID), "token_prefix", prefix)
	return deviceCreds(d.ProductID, d.DeviceID, token, p.Cfg.BrokerHostPublic, d.TopicStyle), nil
}

func (p *Platform) UpdateDevice(id int64, in DeviceInput) (*models.Device, error) {
	d, err := p.GetDevice(id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		d.Name = in.Name
	}
	d.Description = in.Description
	if in.Tags != nil {
		d.Tags = in.Tags
	}
	if in.TopicStyle != "" {
		d.TopicStyle = in.TopicStyle
	}
	_, err = p.DB.Exec(`UPDATE devices SET name=?, description=?, tags=?, topic_style=?, updated_at=? WHERE id=?`,
		d.Name, d.Description, tagsJSON(d.Tags), d.TopicStyle, db.NowMS(), id)
	if err != nil {
		return nil, err
	}
	return p.GetDevice(id)
}

func (p *Platform) SetDeviceEnabled(id int64, enabled bool) error {
	d, err := p.GetDevice(id)
	if err != nil {
		return err
	}
	v := 0
	if enabled {
		v = 1
	}
	_, err = p.DB.Exec(`UPDATE devices SET enabled=?, updated_at=? WHERE id=?`, v, db.NowMS(), id)
	if err != nil {
		return err
	}
	if !enabled && p.Broker != nil {
		// Keep MQTT user; device is rejected by platform maps when disabled.
		_ = d
	}
	_ = p.ReloadDeviceMaps()
	return nil
}

func (p *Platform) DeleteDevice(id int64) error {
	d, err := p.GetDevice(id)
	if err != nil {
		return err
	}
	if p.Broker != nil {
		_ = p.Broker.DeleteUser(broker.DeviceMQTTUsername(d.ProductID, d.DeviceID))
	}
	res, err := p.DB.Exec(`DELETE FROM devices WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("not found")
	}
	_ = p.ReloadDeviceMaps()
	return nil
}

func deviceCreds(productID, deviceID, token, host, style string) *models.DeviceCredentials {
	user := broker.DeviceMQTTUsername(productID, deviceID)
	return &models.DeviceCredentials{
		ProductID:   productID,
		DeviceID:    deviceID,
		AccessToken: token,
		Username:    user,
		ClientID:    user,
		BrokerHost:  host,
		BrokerPort:  1883,
		TopicStyle:  style,
		Snippet:     snippet(productID, deviceID, host, token),
	}
}

func (p *Platform) MarkOnline(d *models.Device) {
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET status='online', last_seen_at=?, updated_at=? WHERE id=?`, now, now, d.ID)
	d.Status = "online"
	d.LastSeenAt = &now
	p.emit("device.status", map[string]any{"id": d.ID, "product_id": d.ProductID, "device_id": d.DeviceID, "status": "online", "ts": now})
	p.publishRules(d.ProductID, d.DeviceID, "status", map[string]any{"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "online": true})
	_ = p.RecordEvent(d, "", "status", "info", "online", "Device online", now, nil)
}

func (p *Platform) MarkOffline(d *models.Device, reason string) {
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET status='offline', updated_at=? WHERE id=?`, now, d.ID)
	d.Status = "offline"
	p.emit("device.status", map[string]any{"id": d.ID, "product_id": d.ProductID, "device_id": d.DeviceID, "status": "offline", "reason": reason, "ts": now})
	p.publishRules(d.ProductID, d.DeviceID, "status", map[string]any{"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "online": false, "reason": reason})
	msg := "Device offline"
	if reason != "" {
		msg = "Device offline (" + reason + ")"
	}
	_ = p.RecordEvent(d, "", "status", "warning", "offline", msg, now, map[string]any{"reason": reason})
}

func (p *Platform) TouchSeen(d *models.Device) {
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET last_seen_at=?, status='online', updated_at=? WHERE id=?`, now, now, d.ID)
}

func (p *Platform) Overview() (models.Overview, error) {
	var o models.Overview
	_ = p.DB.QueryRow(`SELECT
		SUM(CASE WHEN status='online' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status='offline' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status='unknown' THEN 1 ELSE 0 END),
		COUNT(1) FROM devices`).Scan(&o.OnlineDevices, &o.OfflineDevices, &o.UnknownDevices, &o.TotalDevices)
	cutoff := db.NowMS() - 24*3600*1000
	_ = p.DB.QueryRow(`SELECT COUNT(1) FROM events WHERE ts >= ?`, cutoff).Scan(&o.RecentEvents)
	o.PlatformName = p.Cfg.PlatformName
	o.NodeRedURL = p.Cfg.NodeRedURL
	if p.Pub != nil {
		o.MQTTConnected = p.Pub.Connected()
	}
	return o, nil
}
