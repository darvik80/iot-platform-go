package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/plugins"
	"cogitor-iot-platform/internal/protocol"
)

var idRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)

type Publisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) error
	Connected() bool
}

// MQTTSession drops live broker sessions and exposes connection status.
type MQTTSession interface {
	DropUsername(username string)
	Status() models.MQTTStatus
}

type Hub interface {
	Broadcast(event string, data any)
}

type Platform struct {
	DB   *sql.DB
	Cfg  config.Config
	MQTT MQTTSession
	Pub  Publisher
	Hub  Hub
	Log  *slog.Logger

	mu         sync.RWMutex
	byClientID map[string]*models.Device // product.device
	byToken    map[string]*models.Device // hash -> device
	rpcWaiters map[string]chan models.RPCCall
	rpcSeq     uint32

	Plugins         *PluginHub
	ExternalPlugins *ExternalPluginHub
}

func New(database *sql.DB, cfg config.Config, log *slog.Logger) *Platform {
	p := &Platform{
		DB:         database,
		Cfg:        cfg,
		Log:        log,
		byClientID: map[string]*models.Device{},
		byToken:    map[string]*models.Device{},
		rpcWaiters: map[string]chan models.RPCCall{},
	}
	p.Plugins = newPluginHub(p)
	p.ExternalPlugins = newExternalPluginHub(p)
	plugins.SetDefaultTZ(cfg.TimezoneOrUTC())
	return p
}

func (p *Platform) SetPublisher(pub Publisher)   { p.Pub = pub }
func (p *Platform) SetMQTTSession(s MQTTSession) { p.MQTT = s }
func (p *Platform) SetHub(h Hub)                 { p.Hub = h }

const deviceSelect = `SELECT d.id, d.product_pk, pr.product_id, d.device_id, d.name, d.description, d.tags,
	       d.access_token_hash, d.token_prefix, d.status, d.last_seen_at, d.last_ip, d.fw_version,
	       d.topic_style, COALESCE(d.auto_ota,1), d.enabled, d.created_at, d.updated_at
	FROM devices d JOIN products pr ON pr.id = d.product_pk`

func (p *Platform) ReloadDeviceMaps() error {
	rows, err := p.DB.Query(deviceSelect + ` WHERE d.enabled = 1 AND d.access_token_hash IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	byClient := map[string]*models.Device{}
	byToken := map[string]*models.Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return err
		}
		byClient[d.ProductID+"."+d.DeviceID] = d
		if d.AccessTokenHash != "" {
			byToken[d.AccessTokenHash] = d
		}
	}
	p.mu.Lock()
	p.byClientID = byClient
	p.byToken = byToken
	p.mu.Unlock()
	return nil
}

func (p *Platform) ResolveByClientID(clientID string) *models.Device {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.byClientID[clientID]
}

func (p *Platform) ResolveByToken(token string) *models.Device {
	h := auth.HashToken(token)
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.byToken[h]
}

func (p *Platform) emit(event string, data any) {
	if p.Hub != nil {
		p.Hub.Broadcast(event, data)
	}
}

func (p *Platform) publishRules(productID, deviceID, kind string, payload any) {
	if p.Plugins != nil {
		p.Plugins.Dispatch(productID, deviceID, kind, payload)
	}
	if p.Pub == nil || !p.Cfg.RulesBusEnabled {
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	topic := fmt.Sprintf("%s/events/%s/%s/%s", p.Cfg.RulesBusPrefix, productID, deviceID, kind)
	_ = p.Pub.Publish(topic, 0, false, b)
}

// ObserveTopicStyle stores the MQTT dialect inferred from Will / subscribe / publish prefixes.
func (p *Platform) ObserveTopicStyle(d *models.Device, style string) {
	if d == nil || !protocol.ValidStyle(style) {
		return
	}
	if d.TopicStyle == style {
		return
	}
	now := db.NowMS()
	_, _ = p.DB.Exec(`UPDATE devices SET topic_style=?, updated_at=? WHERE id=?`, style, now, d.ID)
	d.TopicStyle = style
	if p.Log != nil {
		p.Log.Info("topic style detected", "product", d.ProductID, "device", d.DeviceID, "style", style)
	}
	p.emit("mqtt", map[string]any{
		"product_id": d.ProductID, "device_id": d.DeviceID, "topic_style": style,
	})
}

func (p *Platform) DownTopic(d *models.Device, capability, operation string) string {
	return protocol.DownTopic(d.TopicStyle, d.ProductID, d.DeviceID, capability, operation)
}

func ValidateIDs(productID, deviceID string) error {
	productID = strings.TrimSpace(productID)
	deviceID = strings.TrimSpace(deviceID)
	if !idRe.MatchString(productID) {
		return errors.New("invalid product_id: use [a-zA-Z0-9][a-zA-Z0-9._-]{0,62}")
	}
	if !idRe.MatchString(deviceID) {
		return errors.New("invalid device_id: use [a-zA-Z0-9][a-zA-Z0-9._-]{0,62}")
	}
	return nil
}

func fingerprint(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:16])
}

func (p *Platform) SeenRecently(devicePK int64, capability string, payload []byte, window time.Duration) bool {
	fp := fingerprint(payload)
	now := db.NowMS()
	var seenAt int64
	err := p.DB.QueryRow(`SELECT seen_at FROM ingest_dedup WHERE device_pk=? AND capability=? AND fingerprint=?`,
		devicePK, capability, fp).Scan(&seenAt)
	if err == nil && now-seenAt < window.Milliseconds() {
		return true
	}
	_, _ = p.DB.Exec(`INSERT INTO ingest_dedup(device_pk, capability, fingerprint, seen_at) VALUES(?,?,?,?)
		ON CONFLICT(device_pk, capability, fingerprint) DO UPDATE SET seen_at=excluded.seen_at`,
		devicePK, capability, fp, now)
	return false
}

func (p *Platform) Audit(userID *int64, action, entityType, entityID string, detail any) {
	var dj *string
	if detail != nil {
		b, _ := json.Marshal(detail)
		s := string(b)
		dj = &s
	}
	_, _ = p.DB.Exec(`INSERT INTO audit_log(user_id, action, entity_type, entity_id, detail_json, created_at) VALUES(?,?,?,?,?,?)`,
		userID, action, entityType, entityID, dj, db.NowMS())
}

func (p *Platform) RetentionLoop(ctx context.Context) {
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	for {
		p.runRetention()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (p *Platform) runRetention() {
	days := p.Cfg.TelemetryRetentionDays
	if days <= 0 {
		return
	}
	cutoff := db.NowMS() - int64(days)*24*3600*1000
	_, _ = p.DB.Exec(`DELETE FROM telemetry_samples WHERE ts < ?`, cutoff)
	_, _ = p.DB.Exec(`DELETE FROM events WHERE ts < ?`, cutoff)
	_, _ = p.DB.Exec(`DELETE FROM device_logs WHERE ts < ?`, cutoff)
	_, _ = p.DB.Exec(`DELETE FROM ingest_dedup WHERE seen_at < ?`, cutoff)
	_, _ = p.DB.Exec(`DELETE FROM attribute_history WHERE updated_at < ?`, cutoff)
}

func snippet(product, device, host, token string) string {
	return fmt.Sprintf(`CogitorCredentials::createAccessToken("%s", "%s", "%s", "%s");`, product, device, host, token)
}

func nullInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func parseTags(s string) []string {
	var tags []string
	if s == "" {
		return []string{}
	}
	_ = json.Unmarshal([]byte(s), &tags)
	if tags == nil {
		return []string{}
	}
	return tags
}

func tagsJSON(tags []string) string {
	if tags == nil {
		tags = []string{}
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func scanDevice(row interface{ Scan(dest ...any) error }) (*models.Device, error) {
	var d models.Device
	var tags string
	var tokenHash sql.NullString
	var lastSeen sql.NullInt64
	var enabled, autoOTA int
	err := row.Scan(&d.ID, &d.ProductPK, &d.ProductID, &d.DeviceID, &d.Name, &d.Description, &tags,
		&tokenHash, &d.TokenPrefix, &d.Status, &lastSeen, &d.LastIP, &d.FwVersion, &d.TopicStyle, &autoOTA, &enabled, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	d.Tags = parseTags(tags)
	d.AccessTokenHash = tokenHash.String
	d.LastSeenAt = nullInt64(lastSeen)
	d.AutoOTA = autoOTA == 1
	d.Enabled = enabled == 1
	return &d, nil
}

func (p *Platform) nextRPCID() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rpcSeq++
	if p.rpcSeq == 0 {
		p.rpcSeq = 1
	}
	return p.rpcSeq
}

func jsonValue(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (p *Platform) getSetting(key, fallback string) string {
	var val string
	err := p.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&val)
	if err != nil || val == "" {
		return fallback
	}
	return val
}

func (p *Platform) setSetting(key, val string) {
	_, _ = p.DB.Exec(`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, val)
}
