package services

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"unicode"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
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
	AlinkSecret string   `json:"alink_secret"`
	AutoOTA     *bool    `json:"auto_ota"`
}

func (p *Platform) ListDevices(accountID int64, productID, status, q string) ([]models.Device, error) {
	query := deviceSelect + ` WHERE 1=1`
	args := []any{}
	if accountID > 0 {
		query += ` AND pr.account_id=?`
		args = append(args, accountID)
	}
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
	row := p.DB.QueryRow(deviceSelect+` WHERE d.id=?`, id)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("not found")
	}
	return d, err
}

// GetDeviceScoped returns a device only if it belongs to the given account (via product).
// Pass accountID=0 to skip the account check (internal callers).
func (p *Platform) GetDeviceScoped(accountID, id int64) (*models.Device, error) {
	d, err := p.GetDevice(id)
	if err != nil {
		return nil, err
	}
	if accountID > 0 {
		pr, err := p.GetProduct(d.ProductPK)
		if err != nil {
			return nil, errors.New("not found")
		}
		if pr.AccountID != accountID {
			return nil, errors.New("not found")
		}
	}
	return d, nil
}

// GetDeviceByAccountIdentity looks a device up inside one account only.
func (p *Platform) GetDeviceByAccountIdentity(accountSlug, productID, deviceID string) (*models.Device, error) {
	row := p.DB.QueryRow(deviceSelect+` WHERE a.slug=? AND pr.product_id=? AND d.device_id=?`, accountSlug, productID, deviceID)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("not found")
	}
	return d, err
}

func (p *Platform) GetDeviceByIdentity(productID, deviceID string) (*models.Device, error) {
	row := p.DB.QueryRow(deviceSelect+` WHERE pr.product_id=? AND d.device_id=?`, productID, deviceID)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("not found")
	}
	return d, err
}

// CreateDevice adds a device to a product of accountID. The MQTT username
// product.device must stay unique across the whole broker, so the pair is
// rejected if any account already uses it.
func (p *Platform) CreateDevice(accountID int64, in DeviceInput) (*models.Device, *models.DeviceCredentials, error) {
	in.ProductID = strings.TrimSpace(in.ProductID)
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.Name = strings.TrimSpace(in.Name)
	if err := ValidateIDs(in.ProductID, in.DeviceID); err != nil {
		return nil, nil, err
	}
	pr, err := p.GetProductBySlugInAccount(accountID, in.ProductID)
	if err != nil {
		return nil, nil, fmt.Errorf("product: %w", err)
	}
	var taken int
	_ = p.DB.QueryRow(`SELECT COUNT(1) FROM devices d JOIN products pr ON pr.id=d.product_pk
		WHERE pr.product_id=? AND d.device_id=?`, in.ProductID, in.DeviceID).Scan(&taken)
	if taken > 0 {
		return nil, nil, fmt.Errorf("device %s.%s already exists on this platform", in.ProductID, in.DeviceID)
	}
	if in.Name == "" {
		in.Name = in.DeviceID
	}
	style := in.TopicStyle
	if style == "" {
		style = pr.TopicStyleDefault
	}
	if !protocol.ValidStyle(style) {
		return nil, nil, fmt.Errorf("invalid topic_style %q", style)
	}

	now := db.NowMS()
	var credential string
	usesSecret := protocol.DialectFor(style).UsesSecret()
	if usesSecret {
		// one credential per device: secret-carrying dialects have no
		// access token; the raw secret is stored because auth needs it
		credential = strings.TrimSpace(in.AlinkSecret)
		if credential == "" {
			credential, err = auth.GenerateAccessToken()
			if err != nil {
				return nil, nil, err
			}
		}
	} else {
		credential, err = auth.GenerateAccessToken()
		if err != nil {
			return nil, nil, err
		}
	}

	var res sql.Result
	var hash, prefix string
	if usesSecret {
		res, err = p.DB.Exec(`INSERT INTO devices(product_pk, device_id, name, description, tags, status, topic_style, alink_secret, enabled, created_at, updated_at)
			VALUES(?,?,?,?,?,?,'alink',?,1,?,?)`,
			pr.ID, in.DeviceID, in.Name, in.Description, tagsJSON(in.Tags), "offline", credential, now, now)
	} else {
		hash = auth.HashToken(credential)
		prefix = auth.TokenPrefix(credential)
		res, err = p.DB.Exec(`INSERT INTO devices(product_pk, device_id, name, description, tags, access_token_hash, token_prefix, status, topic_style, enabled, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,1,?,?)`,
			pr.ID, in.DeviceID, in.Name, in.Description, tagsJSON(in.Tags), hash, prefix, "offline", style, now, now)
	}
	if err != nil {
		return nil, nil, err
	}
	id, _ := res.LastInsertId()
	if !usesSecret {
		if _, err := p.DB.Exec(`INSERT INTO device_credentials(device_pk, account_id, token_hash, token_prefix, created_at) VALUES(?,?,?,?,?)`,
			id, pr.AccountID, hash, prefix, now); err != nil && p.Log != nil {
			p.Log.Warn("failed to insert device credential history", "device_id", id, "err", err)
		}
	}
	_ = p.ReloadDeviceMaps()
	d, err := p.GetDevice(id)
	if err != nil {
		return nil, nil, err
	}
	creds := p.deviceCreds(in.ProductID, in.DeviceID, credential, style)
	if p.Log != nil {
		p.Log.Info("device created", "product", in.ProductID, "device", in.DeviceID, "mqtt_user", creds.Username, "token_prefix", prefix)
	}
	return d, creds, nil
}

func (p *Platform) RotateDeviceToken(id int64) (*models.DeviceCredentials, error) {
	d, err := p.GetDevice(id)
	if err != nil {
		return nil, err
	}
	now := db.NowMS()
	credential, err := auth.GenerateAccessToken()
	if err != nil {
		return nil, err
	}
	dl := protocol.DialectFor(d.TopicStyle)
	// Drop live sessions before the credential changes: DropDevice resolves
	// sessions through the in-memory maps, which still hold the old token.
	if p.MQTT != nil {
		p.MQTT.DropDevice(id)
	}
	if dl.UsesSecret() {
		if _, err := p.DB.Exec(`UPDATE devices SET alink_secret=?, updated_at=? WHERE id=?`, credential, now, id); err != nil {
			return nil, err
		}
	} else {
		hash := auth.HashToken(credential)
		prefix := auth.TokenPrefix(credential)
		if _, err := p.DB.Exec(`UPDATE device_credentials SET revoked_at=? WHERE device_pk=? AND revoked_at IS NULL`, now, id); err != nil && p.Log != nil {
			p.Log.Warn("failed to revoke old device credentials", "device_id", id, "err", err)
		}
		_, err = p.DB.Exec(`UPDATE devices SET access_token_hash=?, token_prefix=?, updated_at=? WHERE id=?`, hash, prefix, now, id)
		if err != nil {
			return nil, err
		}
		if _, err := p.DB.Exec(`INSERT INTO device_credentials(device_pk, account_id, token_hash, token_prefix, created_at) VALUES(?,?,?,?,?)`, id, d.AccountID, hash, prefix, now); err != nil && p.Log != nil {
			p.Log.Warn("failed to insert device credential history", "device_id", id, "err", err)
		}
	}
	_ = p.ReloadDeviceMaps()
	if p.Log != nil {
		args := []any{"product", d.ProductID, "device", d.DeviceID, "style", d.TopicStyle}
		if !dl.UsesSecret() {
			args = append(args, "token_prefix", auth.TokenPrefix(credential))
		}
		p.Log.Info("device credential rotated", args...)
	}
	return p.deviceCreds(d.ProductID, d.DeviceID, credential, d.TopicStyle), nil
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
	if in.AutoOTA != nil {
		d.AutoOTA = *in.AutoOTA
	}
	styleChanged := false
	if in.TopicStyle != "" && in.TopicStyle != d.TopicStyle {
		if !protocol.ValidStyle(in.TopicStyle) {
			return nil, fmt.Errorf("invalid topic_style %q", in.TopicStyle)
		}
		styleChanged = true
	}
	secret := strings.TrimSpace(in.AlinkSecret)
	newStyle := d.TopicStyle
	if styleChanged {
		newStyle = in.TopicStyle
		if protocol.DialectFor(newStyle).UsesSecret() && secret == "" && d.AlinkSecret == "" {
			return nil, errors.New("alink_secret required when switching topic_style to alink (or rotate after the switch)")
		}
	}
	autoOTA := 0
	if d.AutoOTA {
		autoOTA = 1
	}
	_, err = p.DB.Exec(`UPDATE devices SET name=?, description=?, tags=?, auto_ota=?, updated_at=? WHERE id=?`,
		d.Name, d.Description, tagsJSON(d.Tags), autoOTA, db.NowMS(), id)
	if err != nil {
		return nil, err
	}
	if styleChanged {
		// drop the old style's sessions — switching back requires a fresh
		// rotate, no silent credential resurrection. Dropping before the
		// UPDATE keeps the old usernames/token resolvable.
		if p.MQTT != nil {
			p.MQTT.DropDevice(id)
		}
		if protocol.DialectFor(newStyle).UsesSecret() {
			_, _ = p.DB.Exec(`UPDATE devices SET topic_style=?, access_token_hash=NULL, token_prefix='', updated_at=? WHERE id=?`, newStyle, db.NowMS(), id)
			now := db.NowMS()
			_, _ = p.DB.Exec(`UPDATE device_credentials SET revoked_at=? WHERE device_pk=? AND revoked_at IS NULL`, now, id)
		} else {
			_, _ = p.DB.Exec(`UPDATE devices SET topic_style=?, alink_secret=NULL, updated_at=? WHERE id=?`, newStyle, db.NowMS(), id)
		}
	}
	if protocol.DialectFor(newStyle).UsesSecret() && secret != "" {
		_, err = p.DB.Exec(`UPDATE devices SET alink_secret=?, updated_at=? WHERE id=?`, secret, db.NowMS(), id)
		if err != nil {
			return nil, err
		}
	}
	if styleChanged || secret != "" {
		_ = p.ReloadDeviceMaps()
	}
	return p.GetDevice(id)
}

func (p *Platform) SetDeviceEnabled(id int64, enabled bool) error {
	if _, err := p.GetDevice(id); err != nil {
		return err
	}
	v := 0
	if enabled {
		v = 1
	}
	// Dropping before the UPDATE keeps the device resolvable: a disabled
	// device is removed from the credential maps on reload.
	if !enabled && p.MQTT != nil {
		p.MQTT.DropDevice(id)
	}
	_, err := p.DB.Exec(`UPDATE devices SET enabled=?, updated_at=? WHERE id=?`, v, db.NowMS(), id)
	if err != nil {
		return err
	}
	_ = p.ReloadDeviceMaps()
	return nil
}

func (p *Platform) DeleteDevice(id int64) error {
	if _, err := p.GetDevice(id); err != nil {
		return err
	}
	if p.MQTT != nil {
		p.MQTT.DropDevice(id)
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

func (p *Platform) deviceCreds(productID, deviceID, token, style string) *models.DeviceCredentials {
	host := p.Cfg.Platform.BrokerHostPublic
	port := p.Cfg.MQTT.PublicPort()
	wsURL := p.Cfg.MQTT.WSPublicURL(host)
	wsPort := p.Cfg.MQTT.WSPublicPort()
	tlsEnabled := p.Cfg.MQTT.TLSEnabled()
	if tlsEnabled {
		port = p.Cfg.MQTT.TLSPublicPort()
	}
	c := &models.DeviceCredentials{
		ProductID:    productID,
		DeviceID:     deviceID,
		AccessToken:  token,
		BrokerHost:   host,
		BrokerPort:   port,
		BrokerWSURL:  wsURL,
		BrokerWSPort: wsPort,
		TopicStyle:   style,
		Provision: &models.DeviceProvision{
			WiFi: models.DeviceProvisionWiFi{},
			Cogitor: models.DeviceProvisionCogitor{
				Product:    productID,
				Device:     deviceID,
				Host:       host,
				Port:       port,
				WSURL:      wsURL,
				WSPort:     wsPort,
				Token:      token,
				TLS:        tlsEnabled,
				TopicShort: style == protocol.StyleShort,
			},
		},
	}
	dl := protocol.DialectFor(style)
	user := dl.CredsUsername(productID, deviceID, token)
	c.Username = user
	c.ClientID = user
	if dl.UsesSecret() {
		c.DeviceSecret = token
	}
	c.Snippet = dl.Snippet(productID, deviceID, host, token)
	return c
}

func (p *Platform) MarkOnline(d *models.Device) {
	if d.Status == "online" {
		return
	}
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET status='online', last_seen_at=?, updated_at=? WHERE id=?`, now, now, d.ID)
	d.Status = "online"
	d.LastSeenAt = &now
	p.emit(d.AccountID, "device.status", map[string]any{"id": d.ID, "product_id": d.ProductID, "device_id": d.DeviceID, "status": "online", "ts": now})
	p.publishRules(d.AccountSlug, d.ProductID, d.DeviceID, protocol.BusStatus, map[string]any{"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "online": true})
	_ = p.RecordEvent(d, "", "status", "info", "online", "Device online", now, nil)
	p.flushPendingOTA(d)
	_ = p.maybeAutoOTA(d, d.FwVersion, "main")
}

func (p *Platform) MarkOffline(d *models.Device, reason string) {
	if d.Status == "offline" {
		return
	}
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET status='offline', updated_at=? WHERE id=?`, now, d.ID)
	d.Status = "offline"
	p.emit(d.AccountID, "device.status", map[string]any{"id": d.ID, "product_id": d.ProductID, "device_id": d.DeviceID, "status": "offline", "reason": reason, "ts": now})
	p.publishRules(d.AccountSlug, d.ProductID, d.DeviceID, protocol.BusStatus, map[string]any{"ts": now, "product_id": d.ProductID, "device_id": d.DeviceID, "online": false, "reason": reason})
	msg := "Device offline"
	if reason != "" {
		msg = "Device offline (" + reason + ")"
	}
	_ = p.RecordEvent(d, "", "status", "warning", "offline", msg, now, map[string]any{"reason": reason})
}

func (p *Platform) RememberMQTTPeer(d *models.Device, remote string) {
	if d == nil || remote == "" {
		return
	}
	host, _, err := net.SplitHostPort(remote)
	if err == nil {
		remote = host
	}
	d.LastIP = remote
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET last_ip=?, last_seen_at=?, updated_at=? WHERE id=?`, remote, now, now, d.ID)
}

func (p *Platform) MQTTStatus() models.MQTTStatus {
	if p.MQTT == nil {
		return models.MQTTStatus{Clients: []models.MQTTClient{}, Events: []models.MQTTEvent{}}
	}
	return p.MQTT.Status()
}

func (p *Platform) MQTTClientByUser(username string) *models.MQTTClient {
	if username == "" {
		return nil
	}
	st := p.MQTTStatus()
	for i := range st.Clients {
		if st.Clients[i].Username == username || st.Clients[i].ClientID == username {
			c := st.Clients[i]
			return &c
		}
	}
	return nil
}

// MQTTClientByTokenPrefix finds a live session whose username starts with the
// access-token prefix. ThingsBoard sessions connect with the raw token as the
// username; the token itself is never stored in plaintext.
func (p *Platform) MQTTClientByTokenPrefix(prefix string) *models.MQTTClient {
	if prefix == "" {
		return nil
	}
	st := p.MQTTStatus()
	for i := range st.Clients {
		if strings.HasPrefix(st.Clients[i].Username, prefix) {
			c := st.Clients[i]
			return &c
		}
	}
	return nil
}

func (p *Platform) TouchSeen(d *models.Device) {
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET last_seen_at=?, status='online', updated_at=? WHERE id=?`, now, now, d.ID)
}

func (p *Platform) Overview(accountID int64) (models.Overview, error) {
	var o models.Overview
	if accountID > 0 {
		_ = p.DB.QueryRow(`SELECT
			SUM(CASE WHEN d.status='online' THEN 1 ELSE 0 END),
			SUM(CASE WHEN d.status='offline' THEN 1 ELSE 0 END),
			SUM(CASE WHEN d.status='unknown' THEN 1 ELSE 0 END),
			COUNT(1) FROM devices d JOIN products pr ON pr.id = d.product_pk WHERE pr.account_id=?`, accountID).Scan(&o.OnlineDevices, &o.OfflineDevices, &o.UnknownDevices, &o.TotalDevices)
		cutoff := db.NowMS() - 24*3600*1000
		_ = p.DB.QueryRow(`SELECT COUNT(1) FROM events WHERE ts >= ? AND account_id=?`, cutoff, accountID).Scan(&o.RecentEvents)
	} else {
		_ = p.DB.QueryRow(`SELECT
			SUM(CASE WHEN status='online' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status='offline' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status='unknown' THEN 1 ELSE 0 END),
			COUNT(1) FROM devices`).Scan(&o.OnlineDevices, &o.OfflineDevices, &o.UnknownDevices, &o.TotalDevices)
		cutoff := db.NowMS() - 24*3600*1000
		_ = p.DB.QueryRow(`SELECT COUNT(1) FROM events WHERE ts >= ?`, cutoff).Scan(&o.RecentEvents)
	}
	o.PlatformName = p.Cfg.Platform.Name
	if p.MQTT != nil {
		st := p.MQTT.Status()
		o.MQTTConnected = st.Listening
		o.MQTTClients = st.ClientCount
	} else if p.Pub != nil {
		o.MQTTConnected = p.Pub.Connected()
	}
	return o, nil
}
