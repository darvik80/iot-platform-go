package services

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	netmail "net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/mail"
	"cogitor-iot-platform/internal/models"
)

const (
	monitorMaxRules    = 100
	monitorMaxChannels = 20
	monitorMaxAttempts = 5
	monitorLeaseMS     = int64(60000)
)

var (
	monitorTokenPattern = regexp.MustCompile(`^[0-9]{1,20}:[A-Za-z0-9_-]{20,128}$`)
	monitorChatPattern  = regexp.MustCompile(`^(-?[0-9]{1,20}|@[A-Za-z][A-Za-z0-9_]{4,63})$`)
)

type monitorStoredRule struct {
	models.AlertRule
	createdAt, lastNotified, lastEvaluated, revision int64
}

type monitorStoredChannel struct {
	models.NotificationChannel
	ciphertext string
}

const monitorRuleColumns = `id, account_id, name, device_id, metric, kind, operator, threshold, recovery,
	for_seconds, missing_seconds, cooldown_seconds, enabled, channel_ids, state, since, created_at, last_notified, last_evaluated, revision`
const monitorChannelColumns = `id, account_id, name, kind, destination, enabled, secret_ciphertext`

func scanMonitorRule(row interface{ Scan(...any) error }) (monitorStoredRule, error) {
	var r monitorStoredRule
	var enabled int
	var channels string
	err := row.Scan(&r.ID, &r.AccountID, &r.Name, &r.DeviceID, &r.Metric, &r.Kind, &r.Operator, &r.Threshold, &r.Recovery,
		&r.ForSeconds, &r.MissingSeconds, &r.CooldownSeconds, &enabled, &channels, &r.State, &r.Since, &r.createdAt, &r.lastNotified, &r.lastEvaluated, &r.revision)
	if err != nil {
		return r, err
	}
	r.Enabled = enabled == 1
	err = json.Unmarshal([]byte(channels), &r.ChannelIDs)
	if r.ChannelIDs == nil {
		r.ChannelIDs = []int64{}
	}
	return r, err
}

func scanMonitorChannel(row interface{ Scan(...any) error }) (monitorStoredChannel, error) {
	var c monitorStoredChannel
	var enabled int
	err := row.Scan(&c.ID, &c.AccountID, &c.Name, &c.Kind, &c.Destination, &enabled, &c.ciphertext)
	c.Enabled, c.HasSecret = enabled == 1, c.ciphertext != ""
	return c, err
}

func monitorAlertInvalid(message string) error {
	return fmt.Errorf("%w: %s", ErrMonitorInvalid, message)
}
func monitorBool(v bool) int {
	if v {
		return 1
	}
	return 0
}
func monitorText(s string, max int, required bool) bool {
	return len(s) <= max && (!required || s != "") && strings.IndexFunc(s, unicode.IsControl) < 0
}

// Lock the account row to serialize quota checks and concurrent configuration
// writes on PostgreSQL as well as SQLite. A suspended account cannot send/write.
func monitorLockAccount(tx *db.Tx, accountID int64) error {
	if accountID <= 0 {
		return monitorAlertInvalid("account is required")
	}
	res, err := tx.Exec(`UPDATE accounts SET id=id WHERE id=? AND status='active'`, accountID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMonitorNotFound
	}
	return nil
}

func (p *Platform) ListMonitorRules(accountID int64) ([]models.AlertRule, error) {
	if accountID <= 0 {
		return nil, monitorAlertInvalid("account is required")
	}
	rows, err := p.DB.Query(`SELECT `+monitorRuleColumns+` FROM monitoring_rules WHERE account_id=? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.AlertRule{}
	for rows.Next() {
		r, err := scanMonitorRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r.AlertRule)
	}
	return out, rows.Err()
}

func validateMonitorRule(in *models.AlertRule) error {
	in.Name = strings.TrimSpace(in.Name)
	if !monitorText(in.Name, 120, true) || !monitorText(in.Metric, 128, false) || in.DeviceID <= 0 {
		return monitorAlertInvalid("invalid name, metric or device")
	}
	if in.MissingSeconds == 0 {
		in.MissingSeconds = 300
	}
	if in.CooldownSeconds == 0 {
		in.CooldownSeconds = 300
	}
	if in.ForSeconds < 0 || in.ForSeconds > 604800 || in.MissingSeconds < 1 || in.MissingSeconds > 604800 ||
		in.CooldownSeconds < 60 || in.CooldownSeconds > 604800 {
		return monitorAlertInvalid("durations are out of bounds (cooldown must be at least 60 seconds)")
	}
	if math.IsNaN(in.Threshold) || math.IsInf(in.Threshold, 0) || math.IsNaN(in.Recovery) || math.IsInf(in.Recovery, 0) ||
		math.Abs(in.Threshold) > 1e100 || math.Abs(in.Recovery) > 1e100 {
		return monitorAlertInvalid("threshold and recovery must be finite bounded numbers")
	}
	switch in.Kind {
	case "threshold":
		if in.Metric == "" || (in.Operator != "gt" && in.Operator != "lt") {
			return monitorAlertInvalid("threshold requires a metric and gt/lt operator")
		}
		if (in.Operator == "gt" && in.Recovery > in.Threshold) || (in.Operator == "lt" && in.Recovery < in.Threshold) {
			return monitorAlertInvalid("recovery must provide hysteresis towards normal")
		}
	case "offline":
		if in.Metric != "" {
			return monitorAlertInvalid("offline rules do not use a metric")
		}
		in.Operator, in.Threshold, in.Recovery = "", 0, 0
	case "no_data":
		in.Operator, in.Threshold, in.Recovery = "", 0, 0
	default:
		return monitorAlertInvalid("unknown rule kind")
	}
	if len(in.ChannelIDs) > monitorMaxChannels {
		return monitorAlertInvalid("too many channels")
	}
	seen := map[int64]bool{}
	for _, id := range in.ChannelIDs {
		if id <= 0 || seen[id] {
			return monitorAlertInvalid("invalid or duplicate channel")
		}
		seen[id] = true
	}
	if in.ChannelIDs == nil {
		in.ChannelIDs = []int64{}
	}
	return nil
}

func (p *Platform) SaveMonitorRule(accountID, id int64, in models.AlertRule) (*models.AlertRule, error) {
	if id < 0 {
		return nil, monitorAlertInvalid("invalid rule id")
	}
	if err := validateMonitorRule(&in); err != nil {
		return nil, err
	}
	tx, err := p.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := monitorLockAccount(tx, accountID); err != nil {
		return nil, err
	}
	if id != 0 {
		var found int64
		if err := tx.QueryRow(`SELECT id FROM monitoring_rules WHERE id=? AND account_id=?`, id, accountID).Scan(&found); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrMonitorNotFound
			}
			return nil, err
		}
	} else {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM monitoring_rules WHERE account_id=?`, accountID).Scan(&count); err != nil {
			return nil, err
		}
		if count >= monitorMaxRules {
			return nil, fmt.Errorf("%w: rule limit reached", ErrMonitorConflict)
		}
	}
	var owned int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM devices d JOIN products pr ON pr.id=d.product_pk WHERE d.id=? AND pr.account_id=?`, in.DeviceID, accountID).Scan(&owned); err != nil {
		return nil, err
	}
	if owned != 1 {
		return nil, monitorAlertInvalid("device does not belong to account")
	}
	for _, channelID := range in.ChannelIDs {
		if err := tx.QueryRow(`SELECT COUNT(*) FROM monitoring_channels WHERE id=? AND account_id=?`, channelID, accountID).Scan(&owned); err != nil {
			return nil, err
		}
		if owned != 1 {
			return nil, monitorAlertInvalid("channel does not belong to account")
		}
	}
	channelJSON, _ := json.Marshal(in.ChannelIDs)
	now := db.NowMS()
	args := []any{in.Name, in.DeviceID, in.Metric, in.Kind, in.Operator, in.Threshold, in.Recovery,
		in.ForSeconds, in.MissingSeconds, in.CooldownSeconds, monitorBool(in.Enabled), string(channelJSON), now, now}
	if id == 0 {
		res, err := tx.Exec(`INSERT INTO monitoring_rules(name, device_id, metric, kind, operator, threshold, recovery,
			for_seconds, missing_seconds, cooldown_seconds, enabled, channel_ids, since, updated_at, account_id, created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, append(args, accountID, now)...)
		if err != nil {
			return nil, err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return nil, err
		}
	} else {
		_, err = tx.Exec(`UPDATE monitoring_rules SET name=?, device_id=?, metric=?, kind=?, operator=?, threshold=?, recovery=?,
			for_seconds=?, missing_seconds=?, cooldown_seconds=?, enabled=?, channel_ids=?, since=?, updated_at=?,
			state='normal', last_notified=0, last_evaluated=0, revision=revision+1 WHERE id=? AND account_id=?`, append(args, id, accountID)...)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(`UPDATE monitoring_deliveries SET status='failed', last_error='rule updated', lease_token='', lease_until=0, updated_at=?
			WHERE event_id IN (SELECT id FROM monitoring_alert_events WHERE rule_id=? AND account_id=?) AND status IN ('pending','sending')`, now, id, accountID)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	in.ID, in.AccountID, in.State, in.Since = id, accountID, "normal", now
	return &in, nil
}

func (p *Platform) DeleteMonitorRule(accountID, id int64) error {
	if accountID <= 0 || id <= 0 {
		return monitorAlertInvalid("account and rule are required")
	}
	res, err := p.DB.Exec(`DELETE FROM monitoring_rules WHERE id=? AND account_id=?`, id, accountID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrMonitorNotFound
	}
	return err
}

func (p *Platform) ListMonitorChannels(accountID int64) ([]models.NotificationChannel, error) {
	if accountID <= 0 {
		return nil, monitorAlertInvalid("account is required")
	}
	rows, err := p.DB.Query(`SELECT `+monitorChannelColumns+` FROM monitoring_channels WHERE account_id=? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.NotificationChannel{}
	for rows.Next() {
		c, err := scanMonitorChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c.NotificationChannel)
	}
	return out, rows.Err()
}

func (p *Platform) monitorCipher() (cipher.AEAD, error) {
	if strings.TrimSpace(p.Cfg.Auth.JWTSecret) == "" {
		return nil, monitorAlertInvalid("notification encryption key is not configured")
	}
	mac := hmac.New(sha256.New, []byte(p.Cfg.Auth.JWTSecret))
	_, _ = mac.Write([]byte("cogitor/monitoring/telegram/aes-gcm/v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, errors.New("notification encryption unavailable")
	}
	return cipher.NewGCM(block)
}

func (p *Platform) encryptMonitorSecret(accountID, channelID int64, secret string) (string, error) {
	aead, err := p.monitorCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.New("notification encryption unavailable")
	}
	aad := []byte(fmt.Sprintf("monitoring-channel/v1/%d/%d", accountID, channelID))
	sealed := aead.Seal(nonce, nonce, []byte(secret), aad)
	return "v1:" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (p *Platform) decryptMonitorSecret(c monitorStoredChannel) (string, error) {
	aead, err := p.monitorCipher()
	if err != nil {
		return "", err
	}
	bad := errors.New("notification secret unavailable")
	if !strings.HasPrefix(c.ciphertext, "v1:") {
		return "", bad
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(c.ciphertext, "v1:"))
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return "", bad
	}
	aad := []byte(fmt.Sprintf("monitoring-channel/v1/%d/%d", c.AccountID, c.ID))
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], aad)
	if err != nil || !monitorTokenPattern.Match(plain) {
		return "", bad
	}
	return string(plain), nil
}

func (p *Platform) SaveMonitorChannel(accountID, id int64, in models.NotificationChannelInput) (*models.NotificationChannel, error) {
	in.Name, in.Destination = strings.TrimSpace(in.Name), strings.TrimSpace(in.Destination)
	if id < 0 || !monitorText(in.Name, 120, true) || !monitorText(in.Destination, 254, true) {
		return nil, monitorAlertInvalid("invalid channel name or destination")
	}
	switch in.Kind {
	case "email":
		addr, err := netmail.ParseAddress(in.Destination)
		if err != nil || addr.Address != in.Destination || !strings.Contains(in.Destination, "@") || in.Secret != "" {
			return nil, monitorAlertInvalid("email requires one address and no secret")
		}
	case "telegram":
		if !monitorChatPattern.MatchString(in.Destination) || (in.Secret != "" && !monitorTokenPattern.MatchString(in.Secret)) {
			return nil, monitorAlertInvalid("invalid Telegram destination or token")
		}
		if _, err := p.monitorCipher(); err != nil {
			return nil, err
		}
	default:
		return nil, monitorAlertInvalid("unknown notification channel kind")
	}
	tx, err := p.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := monitorLockAccount(tx, accountID); err != nil {
		return nil, err
	}
	c := monitorStoredChannel{NotificationChannel: in.NotificationChannel}
	c.AccountID = accountID
	now := db.NowMS()
	if id == 0 {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM monitoring_channels WHERE account_id=?`, accountID).Scan(&count); err != nil {
			return nil, err
		}
		if count >= monitorMaxChannels {
			return nil, fmt.Errorf("%w: channel limit reached", ErrMonitorConflict)
		}
		res, err := tx.Exec(`INSERT INTO monitoring_channels(account_id, name, kind, destination, enabled, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`,
			accountID, in.Name, in.Kind, in.Destination, monitorBool(in.Enabled), now, now)
		if err != nil {
			return nil, err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return nil, err
		}
	} else {
		old, err := scanMonitorChannel(tx.QueryRow(`SELECT `+monitorChannelColumns+` FROM monitoring_channels WHERE id=? AND account_id=?`, id, accountID))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrMonitorNotFound
			}
			return nil, err
		}
		if old.Kind == in.Kind {
			c.ciphertext = old.ciphertext
		}
	}
	c.ID = id
	if in.Kind == "telegram" {
		if in.Secret != "" {
			c.ciphertext, err = p.encryptMonitorSecret(accountID, id, in.Secret)
		} else if c.ciphertext == "" {
			err = monitorAlertInvalid("Telegram token is required")
		} else {
			_, err = p.decryptMonitorSecret(c)
		}
		if err != nil {
			return nil, err
		}
	} else {
		c.ciphertext = ""
	}
	_, err = tx.Exec(`UPDATE monitoring_channels SET name=?, kind=?, destination=?, enabled=?, secret_ciphertext=?, updated_at=? WHERE id=? AND account_id=?`,
		in.Name, in.Kind, in.Destination, monitorBool(in.Enabled), c.ciphertext, now, id, accountID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(`UPDATE monitoring_deliveries SET status='failed', last_error='channel updated', lease_token='', lease_until=0, updated_at=?
		WHERE channel_id=? AND account_id=? AND status IN ('pending','sending')`, now, id, accountID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c.HasSecret = c.ciphertext != ""
	return &c.NotificationChannel, nil
}

func (p *Platform) DeleteMonitorChannel(accountID, id int64) error {
	if accountID <= 0 || id <= 0 {
		return monitorAlertInvalid("account and channel are required")
	}
	tx, err := p.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := monitorLockAccount(tx, accountID); err != nil {
		return err
	}
	var found int64
	if err := tx.QueryRow(`SELECT id FROM monitoring_channels WHERE id=? AND account_id=?`, id, accountID).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMonitorNotFound
		}
		return err
	}
	rows, err := tx.Query(`SELECT channel_ids FROM monitoring_rules WHERE account_id=?`, accountID)
	if err != nil {
		return err
	}
	used := false
	for rows.Next() {
		var raw string
		var ids []int64
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			rows.Close()
			return err
		}
		used = used || slices.Contains(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("%w: channel is referenced by a rule", ErrMonitorConflict)
	}
	if _, err := tx.Exec(`DELETE FROM monitoring_channels WHERE id=? AND account_id=?`, id, accountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Platform) TestMonitorChannel(ctx context.Context, accountID, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if accountID <= 0 || id <= 0 {
		return monitorAlertInvalid("account and channel are required")
	}
	c, err := scanMonitorChannel(p.DB.QueryRow(`SELECT `+monitorChannelColumns+` FROM monitoring_channels WHERE id=? AND account_id=? AND enabled=1
		AND EXISTS(SELECT 1 FROM accounts WHERE id=? AND status='active')`, id, accountID, accountID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMonitorNotFound
		}
		return err
	}
	return p.sendMonitorNotification(ctx, c, "Notification channel test", "test", "This is a test notification from Cogitor IoT monitoring.")
}

func monitorHistoryLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}

func (p *Platform) ListMonitorEvents(accountID int64, limit int) ([]models.AlertEvent, error) {
	if accountID <= 0 {
		return nil, monitorAlertInvalid("account is required")
	}
	rows, err := p.DB.Query(`SELECT id, rule_id, name, state, message, created_at FROM monitoring_alert_events WHERE account_id=? ORDER BY id DESC LIMIT ?`, accountID, monitorHistoryLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.AlertEvent{}
	for rows.Next() {
		var e models.AlertEvent
		if err := rows.Scan(&e.ID, &e.RuleID, &e.Name, &e.State, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *Platform) ListMonitorDeliveries(accountID int64, limit int) ([]models.NotificationDelivery, error) {
	if accountID <= 0 {
		return nil, monitorAlertInvalid("account is required")
	}
	rows, err := p.DB.Query(`SELECT id, event_id, channel_id, status, attempts, last_error, next_attempt FROM monitoring_deliveries WHERE account_id=? ORDER BY id DESC LIMIT ?`, accountID, monitorHistoryLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.NotificationDelivery{}
	for rows.Next() {
		var d models.NotificationDelivery
		if err := rows.Scan(&d.ID, &d.EventID, &d.ChannelID, &d.Status, &d.Attempts, &d.LastError, &d.NextAttempt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MonitoringLoop is independent of plugins/device sessions and browser clients.
// Delivery runs separately, so slow SMTP/Telegram cannot stall rule evaluation.
func (p *Platform) MonitoringLoop(ctx context.Context) {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for workerCtx.Err() == nil {
			for i := 0; i < 32 && workerCtx.Err() == nil; i++ {
				worked, err := p.monitorDispatchOnce(workerCtx, db.NowMS(), p.sendMonitorNotification)
				if err != nil && p.Log != nil {
					p.Log.Warn("monitor notification worker failed")
				}
				if !worked || err != nil {
					break
				}
			}
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	defer func() { cancel(); <-done }()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := p.monitorEvaluate(ctx, db.NowMS()); err != nil && p.Log != nil {
			p.Log.Warn("monitor evaluator failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Platform) monitorEvaluate(ctx context.Context, now int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := p.DB.Exec(`UPDATE monitoring_rules SET state='normal', since=?, last_evaluated=0, revision=revision+1
		WHERE state='pending' AND (enabled=0 OR NOT EXISTS(SELECT 1 FROM devices d JOIN products pr ON pr.id=d.product_pk
		JOIN accounts a ON a.id=pr.account_id WHERE d.id=monitoring_rules.device_id AND d.enabled=1 AND a.status='active' AND pr.account_id=monitoring_rules.account_id))`, now); err != nil {
		return err
	}
	// Close each bounded page before opening transactions (SQLite has one conn).
	var after int64
	for ctx.Err() == nil {
		rows, err := p.DB.Query(`SELECT r.id FROM monitoring_rules r JOIN accounts a ON a.id=r.account_id
			JOIN devices d ON d.id=r.device_id JOIN products pr ON pr.id=d.product_pk
			WHERE r.id>? AND r.enabled=1 AND a.status='active' AND d.enabled=1 AND pr.account_id=r.account_id ORDER BY r.id LIMIT 128`, after)
		if err != nil {
			return err
		}
		ids := []int64{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := p.monitorEvaluateRule(ctx, id, now); err != nil {
				return err
			}
			after = id
		}
		if len(ids) < 128 {
			return nil
		}
	}
	return ctx.Err()
}

func (p *Platform) monitorEvaluateRule(ctx context.Context, id, now int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := p.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := scanMonitorRule(tx.QueryRow(`SELECT `+monitorRuleColumns+` FROM monitoring_rules WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !r.Enabled {
		return nil
	}
	var deviceStatus string
	var lastSeen sql.NullInt64
	err = tx.QueryRow(`SELECT d.status, d.last_seen_at FROM devices d JOIN products pr ON pr.id=d.product_pk JOIN accounts a ON a.id=pr.account_id
		WHERE d.id=? AND pr.account_id=? AND d.enabled=1 AND a.status='active'`, r.DeviceID, r.AccountID).Scan(&deviceStatus, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	valid, alarm, recovered := true, false, false
	var value sql.NullFloat64
	if r.Kind == "offline" {
		seen := r.createdAt
		if lastSeen.Valid {
			seen = lastSeen.Int64
		}
		alarm = deviceStatus != "online" || now-seen >= int64(r.MissingSeconds)*1000
		recovered = !alarm
	} else {
		var ts, received int64
		// Presence uses receive time; thresholds use measurement order to reject delayed recovery samples.
		query := `SELECT ts, ingested_at, value_num FROM telemetry_samples WHERE device_pk=? AND account_id=? AND ingested_at<=?`
		args := []any{r.DeviceID, r.AccountID, now}
		if r.Metric != "" {
			query += ` AND key=?`
			args = append(args, r.Metric)
		}
		order := ` ORDER BY ingested_at DESC, id DESC LIMIT 1`
		if r.Kind == "threshold" {
			query += ` AND ts<=?`
			args = append(args, now)
			order = ` ORDER BY ts DESC, ingested_at DESC, id DESC LIMIT 1`
		}
		err = tx.QueryRow(query+order, args...).Scan(&ts, &received, &value)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if r.Kind == "no_data" {
			if received < r.createdAt {
				received = r.createdAt
			}
			alarm = now-received >= int64(r.MissingSeconds)*1000
			recovered = !alarm
		} else {
			freshMS := int64(r.MissingSeconds) * 1000
			valid = err == nil && value.Valid && !math.IsNaN(value.Float64) && !math.IsInf(value.Float64, 0) &&
				ts <= now && ts > now-freshMS && received > now-freshMS
			if valid {
				if r.Operator == "gt" {
					alarm, recovered = value.Float64 > r.Threshold, value.Float64 <= r.Recovery
				} else {
					alarm, recovered = value.Float64 < r.Threshold, value.Float64 >= r.Recovery
				}
			}
		}
	}
	state, since := r.State, r.Since
	if state == "pending" && r.lastEvaluated > 0 && now-r.lastEvaluated > max(int64(r.MissingSeconds)*1000, int64(10000)) {
		state, since = "normal", now
	}
	if !valid {
		// A gap breaks sustain, but must never resolve an already firing rule or
		// re-notify based on an old (or non-numeric) threshold sample.
		if state == "pending" {
			state, since = "normal", now
		}
	} else if state == "firing" {
		if recovered {
			state, since = "resolved", now
		}
	} else if alarm {
		if state != "pending" {
			state, since = "pending", now
		}
		if now-since >= int64(r.ForSeconds)*1000 {
			state, since = "firing", now
		}
	} else if state == "pending" {
		state, since = "normal", now
	}
	transition := state != r.State
	notify := transition && (state == "firing" || state == "resolved")
	if valid && state == "firing" && !transition && now-r.lastNotified >= int64(r.CooldownSeconds)*1000 {
		notify = true
	}
	if !transition && !notify && state != "pending" {
		return nil
	}
	lastNotified := r.lastNotified
	if notify {
		lastNotified = now
	}
	res, err := tx.Exec(`UPDATE monitoring_rules SET state=?, since=?, last_notified=?, last_evaluated=?, revision=revision+1, updated_at=?
		WHERE id=? AND revision=? AND enabled=1
		AND EXISTS(SELECT 1 FROM accounts WHERE id=? AND status='active')
		AND EXISTS(SELECT 1 FROM devices d JOIN products pr ON pr.id=d.product_pk WHERE d.id=? AND d.enabled=1 AND pr.account_id=?)`,
		state, since, lastNotified, now, now, id, r.revision, r.AccountID, r.DeviceID, r.AccountID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	if !transition && !notify {
		return tx.Commit()
	}
	message := fmt.Sprintf("%s: %s (%s, device %d)", r.Name, state, r.Kind, r.DeviceID)
	if r.Metric != "" {
		message += ", metric " + r.Metric
	}
	if r.Kind == "threshold" && valid {
		message += fmt.Sprintf(", value %g", value.Float64)
	}
	res, err = tx.Exec(`INSERT INTO monitoring_alert_events(account_id, rule_id, name, state, message, created_at) VALUES(?,?,?,?,?,?)`, r.AccountID, id, r.Name, state, message, now)
	if err != nil {
		return err
	}
	eventID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if notify {
		for _, channelID := range r.ChannelIDs {
			var owned int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM monitoring_channels WHERE id=? AND account_id=? AND enabled=1`, channelID, r.AccountID).Scan(&owned); err != nil {
				return err
			}
			if owned != 1 {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO monitoring_deliveries(account_id, event_id, channel_id, next_attempt, created_at, updated_at) VALUES(?,?,?,?,?,?)`,
				r.AccountID, eventID, channelID, now, now, now); err != nil {
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

type monitorClaim struct {
	id       int64
	attempts int
	token    string
}

// Claims are compare-and-swap updates, not SELECT followed by an unconditional
// write. Expired claims survive restarts; fencing tokens reject late completions.
func (p *Platform) monitorClaimDelivery(ctx context.Context, now int64) (*monitorClaim, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, err := p.DB.Exec(`UPDATE monitoring_deliveries SET status='failed', last_error='delivery attempt limit reached', lease_token='', lease_until=0, updated_at=?
		WHERE attempts>=? AND (status='pending' OR (status='sending' AND lease_until<=?))`, now, monitorMaxAttempts, now)
	if err != nil {
		return nil, err
	}
	var id int64
	err = p.DB.QueryRow(`SELECT nd.id FROM monitoring_deliveries nd JOIN monitoring_alert_events e ON e.id=nd.event_id WHERE nd.attempts<? AND
		((nd.status='pending' AND nd.next_attempt<=?) OR (nd.status='sending' AND nd.lease_until<=?))
		AND NOT EXISTS(SELECT 1 FROM monitoring_deliveries prior JOIN monitoring_alert_events pe ON pe.id=prior.event_id
		WHERE prior.channel_id=nd.channel_id AND pe.rule_id=e.rule_id AND prior.id<nd.id AND prior.status IN ('pending','sending'))
		ORDER BY nd.next_attempt, nd.id LIMIT 1`, monitorMaxAttempts, now, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.New("notification lease unavailable")
	}
	token := base64.RawURLEncoding.EncodeToString(nonce)
	res, err := p.DB.Exec(`UPDATE monitoring_deliveries SET status='sending', attempts=attempts+1, lease_token=?, lease_until=?, updated_at=?
		WHERE id=? AND attempts<? AND ((status='pending' AND next_attempt<=?) OR (status='sending' AND lease_until<=?))`,
		token, now+monitorLeaseMS, now, id, monitorMaxAttempts, now, now)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return nil, err
	}
	claim := &monitorClaim{id: id, token: token}
	if err := p.DB.QueryRow(`SELECT attempts FROM monitoring_deliveries WHERE id=? AND lease_token=?`, id, token).Scan(&claim.attempts); err != nil {
		return nil, err
	}
	return claim, nil
}

type monitorSendError struct {
	message    string
	retryAfter time.Duration
	permanent  bool
}

func (e *monitorSendError) Error() string { return e.message }

func (p *Platform) monitorFinishDelivery(claim *monitorClaim, now int64, sendErr error) error {
	status, lastError, next := "sent", "", now
	if sendErr != nil {
		// Never persist errors returned by transports: HTTP errors contain the
		// bot token in the request URL; response bodies are also untrusted.
		status, lastError = "pending", "notification delivery failed"
		delay := 30 * time.Second * time.Duration(1<<uint(claim.attempts-1))
		var safe *monitorSendError
		if errors.As(sendErr, &safe) {
			lastError = safe.message
			if safe.retryAfter > delay {
				delay = safe.retryAfter
			}
			if safe.permanent {
				status = "failed"
			}
		}
		if delay > 24*time.Hour {
			delay = 24 * time.Hour
		}
		if claim.attempts >= monitorMaxAttempts {
			status = "failed"
		}
		next = now + delay.Milliseconds()
	}
	_, err := p.DB.Exec(`UPDATE monitoring_deliveries SET status=?, last_error=?, next_attempt=?, lease_until=0, lease_token='', updated_at=?
		WHERE id=? AND status='sending' AND lease_token=?`, status, lastError, next, now, claim.id, claim.token)
	return err
}

type monitorSendFunc func(context.Context, monitorStoredChannel, string, string, string) error

func (p *Platform) monitorDispatchOnce(ctx context.Context, now int64, send monitorSendFunc) (bool, error) {
	started := time.Now()
	claim, err := p.monitorClaimDelivery(ctx, now)
	if err != nil || claim == nil {
		return false, err
	}
	// Revalidate every tenant boundary and enabled flag immediately before send.
	var c monitorStoredChannel
	var enabled int
	var name, state, message, channels string
	err = p.DB.QueryRow(`SELECT c.id, c.account_id, c.name, c.kind, c.destination, c.enabled, c.secret_ciphertext, e.name, e.state, e.message, r.channel_ids
		FROM monitoring_deliveries nd JOIN monitoring_alert_events e ON e.id=nd.event_id AND e.account_id=nd.account_id
		JOIN monitoring_rules r ON r.id=e.rule_id AND r.account_id=nd.account_id
		JOIN monitoring_channels c ON c.id=nd.channel_id AND c.account_id=nd.account_id
		JOIN accounts a ON a.id=nd.account_id JOIN devices d ON d.id=r.device_id JOIN products pr ON pr.id=d.product_pk
		WHERE nd.id=? AND nd.lease_token=? AND nd.status='sending' AND c.enabled=1 AND r.enabled=1 AND a.status='active' AND d.enabled=1 AND pr.account_id=nd.account_id`,
		claim.id, claim.token).Scan(&c.ID, &c.AccountID, &c.Name, &c.Kind, &c.Destination, &enabled, &c.ciphertext, &name, &state, &message, &channels)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return true, err
	}
	var ids []int64
	if err != nil || json.Unmarshal([]byte(channels), &ids) != nil || !slices.Contains(ids, c.ID) {
		err = &monitorSendError{message: "notification target is inactive or unavailable", permanent: true}
	} else {
		c.Enabled, c.HasSecret = enabled == 1, c.ciphertext != ""
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = send(sendCtx, c, name, state, message)
		cancel()
	}
	// Retry-After starts when the attempt completes, not before a slow send.
	return true, p.monitorFinishDelivery(claim, now+time.Since(started).Milliseconds(), err)
}

func (p *Platform) sendMonitorNotification(ctx context.Context, c monitorStoredChannel, name, state, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch c.Kind {
	case "email":
		sender := mail.Sender{Cfg: p.Cfg} // no logging of message content or transport errors
		if err := sender.SendAlert(ctx, c.Destination, name, state, message); err != nil {
			return &monitorSendError{message: "SMTP notification failed"}
		}
		return nil
	case "telegram":
		token, err := p.decryptMonitorSecret(c)
		if err != nil {
			return &monitorSendError{message: "notification secret unavailable", permanent: true}
		}
		return sendMonitorTelegram(ctx, nil, token, c.Destination, name+"\n"+state+"\n"+message)
	default:
		return &monitorSendError{message: "unsupported notification channel", permanent: true}
	}
}

// The endpoint is not configurable. The transport argument is only a test seam;
// production always uses the standard transport, with redirects disabled.
func sendMonitorTelegram(ctx context.Context, transport http.RoundTripper, token, destination, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !monitorTokenPattern.MatchString(token) || !monitorChatPattern.MatchString(destination) {
		return &monitorSendError{message: "invalid Telegram configuration", permanent: true}
	}
	if len(message) > 4000 {
		return &monitorSendError{message: "notification message too long", permanent: true}
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(map[string]any{"chat_id": destination, "text": message, "disable_web_page_preview": true}); err != nil {
		return &monitorSendError{message: "notification encoding failed", permanent: true}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", &body)
	if err != nil {
		return &monitorSendError{message: "Telegram request failed"}
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return &monitorSendError{message: "Telegram request failed"}
	}
	defer resp.Body.Close()
	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	var result struct {
		OK         bool `json:"ok"`
		ErrorCode  int  `json:"error_code"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	decodeErr := json.Unmarshal(payload, &result)
	if readErr == nil && len(payload) <= 64*1024 && decodeErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && result.OK {
		return nil
	}
	delay := monitorRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	if result.Parameters.RetryAfter > 0 {
		seconds := min(result.Parameters.RetryAfter, int64(86400))
		if d := time.Duration(seconds) * time.Second; d > delay {
			delay = d
		}
	}
	code := resp.StatusCode
	if code >= 200 && code < 300 && !result.OK {
		code = result.ErrorCode
	}
	permanent := code >= 400 && code < 500 && code != 408 && code != 429
	return &monitorSendError{message: "Telegram notification rejected", retryAfter: delay, permanent: permanent}
}

func monitorRetryAfter(raw string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil && seconds > 0 {
		return time.Duration(min(seconds, int64(86400))) * time.Second
	}
	if date, err := http.ParseTime(raw); err == nil && date.After(now) {
		return min(date.Sub(now), 24*time.Hour)
	}
	return 0
}
