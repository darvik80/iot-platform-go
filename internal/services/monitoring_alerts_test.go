package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

const alertTestToken = "123456:abcdefghijklmnopqrstuvwxyz_123456789"

func alertFixture(t *testing.T, persistent ...bool) (*Platform, int64, int64) {
	t.Helper()
	cfg := config.Config{Auth: config.AuthConfig{JWTSecret: "test-only-monitoring-encryption-key"}}
	var p *Platform
	if len(persistent) > 0 && persistent[0] {
		p = newTestPlatform(t, cfg)
	} else {
		// Most tests need SQL semantics, not disk I/O. Keep a file-backed test
		// specifically for close/reopen durability below.
		database, err := db.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = database.Close() })
		p = New(database, cfg, slog.New(slog.DiscardHandler))
		alertExec(t, p, `INSERT INTO accounts(id,slug,name,status,created_at,updated_at) VALUES(1,'default','Default','active',?,?)`, db.NowMS(), db.NowMS())
	}
	if _, err := p.CreateProduct(1, ProductInput{ProductID: "alerts", Name: "Alerts"}); err != nil {
		t.Fatal(err)
	}
	d, _, err := p.CreateDevice(1, DeviceInput{ProductID: "alerts", DeviceID: "sensor"})
	if err != nil {
		t.Fatal(err)
	}
	return p, d.ID, db.NowMS()
}

func alertExec(t *testing.T, p *Platform, q string, args ...any) {
	t.Helper()
	if _, err := p.DB.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func alertChannel(t *testing.T, p *Platform) *models.NotificationChannel {
	t.Helper()
	c, err := p.SaveMonitorChannel(1, 0, models.NotificationChannelInput{NotificationChannel: models.NotificationChannel{
		Name: "Operations", Kind: "telegram", Destination: "-123456", Enabled: true,
	}, Secret: alertTestToken})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func alertRule(t *testing.T, p *Platform, deviceID, now int64, kind string, channelIDs ...int64) *models.AlertRule {
	t.Helper()
	r := models.AlertRule{Name: "Temperature alert", DeviceID: deviceID, Kind: kind, Enabled: true,
		MissingSeconds: 120, CooldownSeconds: 60, ChannelIDs: channelIDs}
	if kind == "threshold" {
		r.Metric, r.Operator, r.Threshold, r.Recovery = "temperature", "gt", 30, 25
	}
	saved, err := p.SaveMonitorRule(1, 0, r)
	if err != nil {
		t.Fatal(err)
	}
	alertExec(t, p, `UPDATE monitoring_rules SET created_at=?, since=? WHERE id=?`, now, now, saved.ID)
	return saved
}

func alertSample(t *testing.T, p *Platform, deviceID, ts int64, value any) {
	t.Helper()
	alertExec(t, p, `INSERT INTO telemetry_samples(device_pk, account_id, key, value_num, ts, ingested_at) VALUES(?,1,'temperature',?,?,?)`, deviceID, value, ts, ts)
}

func alertEvaluate(t *testing.T, p *Platform, now int64) {
	t.Helper()
	if err := p.monitorEvaluate(context.Background(), now); err != nil {
		t.Fatal(err)
	}
}

func alertState(t *testing.T, p *Platform, id int64, want string) models.AlertRule {
	t.Helper()
	rules, err := p.ListMonitorRules(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.ID == id {
			if r.State != want {
				t.Fatalf("rule %d state=%s, want %s", id, r.State, want)
			}
			return r
		}
	}
	t.Fatalf("rule %d missing", id)
	return models.AlertRule{}
}

func alertDeliveryCount(t *testing.T, p *Platform, want int) []models.NotificationDelivery {
	t.Helper()
	ds, err := p.ListMonitorDeliveries(1, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != want {
		t.Fatalf("deliveries=%d, want %d: %+v", len(ds), want, ds)
	}
	return ds
}

func TestMonitorAlertThresholdSustainHysteresisCooldownRestart(t *testing.T) {
	p, deviceID, now := alertFixture(t, true)
	c := alertChannel(t, p)
	r := alertRule(t, p, deviceID, now, "threshold", c.ID)
	alertExec(t, p, `UPDATE monitoring_rules SET for_seconds=10 WHERE id=?`, r.ID)
	alertSample(t, p, deviceID, now, 31)
	alertEvaluate(t, p, now)
	first := alertState(t, p, r.ID, "pending")
	alertDeliveryCount(t, p, 0)
	alertEvaluate(t, p, now+9000)
	if got := alertState(t, p, r.ID, "pending"); got.Since != first.Since {
		t.Fatal("pending sustain moved")
	}

	// Reopen the actual file, not just the service object: state is durable.
	if err := p.DB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := db.Open("sqlite", p.Cfg.Database.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	p = New(reopened, p.Cfg, p.Log)
	alertEvaluate(t, p, now+10000)
	alertState(t, p, r.ID, "firing")
	alertDeliveryCount(t, p, 1)
	alertSample(t, p, deviceID, now+20000, 28) // dead band does not resolve
	alertEvaluate(t, p, now+69000)
	alertState(t, p, r.ID, "firing")
	alertDeliveryCount(t, p, 1)
	alertEvaluate(t, p, now+70000)
	alertDeliveryCount(t, p, 2)
	alertEvaluate(t, p, now+70000) // same tick cannot insert duplicate notifications
	alertDeliveryCount(t, p, 2)
	alertSample(t, p, deviceID, now+71000, 25)
	alertEvaluate(t, p, now+71000)
	alertState(t, p, r.ID, "resolved")
	alertDeliveryCount(t, p, 3)
	alertEvaluate(t, p, now+72000)
	alertDeliveryCount(t, p, 3)
	events, err := p.ListMonitorEvents(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[0].State != "resolved" || events[3].State != "pending" {
		t.Fatalf("events: %+v", events)
	}
}

func TestMonitorAlertThresholdFreshnessAndLT(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	c := alertChannel(t, p)
	r := alertRule(t, p, deviceID, now, "threshold", c.ID)
	alertExec(t, p, `UPDATE monitoring_rules SET operator='lt', threshold=10, recovery=15, missing_seconds=20, for_seconds=5 WHERE id=?`, r.ID)
	alertSample(t, p, deviceID, now, 9)
	alertEvaluate(t, p, now)
	alertState(t, p, r.ID, "pending")
	alertEvaluate(t, p, now+20000) // stale data breaks pending sustain
	alertState(t, p, r.ID, "normal")
	alertSample(t, p, deviceID, now+21000, 9)
	alertEvaluate(t, p, now+21000)
	alertEvaluate(t, p, now+26000)
	alertState(t, p, r.ID, "firing")
	alertSample(t, p, deviceID, now+27000, 16)
	alertEvaluate(t, p, now+100000) // recovery value is now stale: do not resolve
	alertState(t, p, r.ID, "firing")
	alertDeliveryCount(t, p, 1)
	alertSample(t, p, deviceID, now+101000, nil) // non-numeric must not resolve either
	alertEvaluate(t, p, now+101000)
	alertState(t, p, r.ID, "firing")
	alertSample(t, p, deviceID, now+102000, 14)
	alertEvaluate(t, p, now+102000)
	alertState(t, p, r.ID, "firing")
	alertSample(t, p, deviceID, now+103000, 15)
	alertEvaluate(t, p, now+103000)
	alertState(t, p, r.ID, "resolved")
}

func TestMonitorAlertNoDataAndOffline(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	noData := alertRule(t, p, deviceID, now, "no_data")
	alertExec(t, p, `UPDATE monitoring_rules SET missing_seconds=30 WHERE id=?`, noData.ID)
	alertEvaluate(t, p, now+29000)
	alertState(t, p, noData.ID, "normal")
	alertEvaluate(t, p, now+30000)
	alertState(t, p, noData.ID, "firing") // no rows uses rule creation time
	alertSample(t, p, deviceID, now+31000, 20)
	alertEvaluate(t, p, now+31000)
	alertState(t, p, noData.ID, "resolved")
	alertExec(t, p, `UPDATE monitoring_rules SET metric='humidity' WHERE id=?`, noData.ID)
	alertEvaluate(t, p, now+32000)
	alertState(t, p, noData.ID, "firing") // another metric does not satisfy this rule
	alertExec(t, p, `UPDATE monitoring_rules SET enabled=0 WHERE id=?`, noData.ID)

	offline := alertRule(t, p, deviceID, now, "offline")
	alertExec(t, p, `UPDATE devices SET status='online', last_seen_at=? WHERE id=?`, now, deviceID)
	alertEvaluate(t, p, now+119000)
	alertState(t, p, offline.ID, "normal")
	alertEvaluate(t, p, now+120000)
	alertState(t, p, offline.ID, "firing") // stale presence even if disconnect was missed
	alertExec(t, p, `UPDATE devices SET last_seen_at=? WHERE id=?`, now+121000, deviceID)
	alertEvaluate(t, p, now+121000)
	alertState(t, p, offline.ID, "resolved")
	alertExec(t, p, `UPDATE devices SET status='offline' WHERE id=?`, deviceID)
	alertEvaluate(t, p, now+122000)
	alertState(t, p, offline.ID, "firing")
}

func TestMonitorAlertDisabledSuspendedAndExecutionIsolation(t *testing.T) {
	for _, scope := range []string{"rule", "device", "account", "device-owner", "channel-owner", "channel-disabled", "channel-removed"} {
		t.Run(scope, func(t *testing.T) {
			p, deviceID, now := alertFixture(t)
			c := alertChannel(t, p)
			r := alertRule(t, p, deviceID, now, "threshold", c.ID)
			alertSample(t, p, deviceID, now, 40)
			alertEvaluate(t, p, now)
			alertDeliveryCount(t, p, 1)
			alertExec(t, p, `INSERT INTO accounts(id,slug,name,status,created_at,updated_at) VALUES(2,'other','Other','active',?,?)`, now, now)
			switch scope {
			case "rule":
				alertExec(t, p, `UPDATE monitoring_rules SET enabled=0 WHERE id=?`, r.ID)
			case "device":
				alertExec(t, p, `UPDATE devices SET enabled=0 WHERE id=?`, deviceID)
			case "account":
				alertExec(t, p, `UPDATE accounts SET status='suspended' WHERE id=1`)
			case "device-owner":
				alertExec(t, p, `UPDATE products SET account_id=2`)
			case "channel-owner":
				alertExec(t, p, `UPDATE monitoring_channels SET account_id=2 WHERE id=?`, c.ID)
			case "channel-disabled":
				alertExec(t, p, `UPDATE monitoring_channels SET enabled=0 WHERE id=?`, c.ID)
			case "channel-removed":
				alertExec(t, p, `UPDATE monitoring_rules SET channel_ids='[]' WHERE id=?`, r.ID)
			}
			calls := 0
			worked, err := p.monitorDispatchOnce(context.Background(), now, func(context.Context, monitorStoredChannel, string, string, string) error { calls++; return nil })
			if err != nil || !worked || calls != 0 {
				t.Fatalf("dispatch worked=%v err=%v calls=%d", worked, err, calls)
			}
			if got := alertDeliveryCount(t, p, 1)[0]; got.Status != "failed" {
				t.Fatalf("delivery %+v", got)
			}
			alertSample(t, p, deviceID, now+1000, 0)
			alertEvaluate(t, p, now+1000)
			if scope == "rule" || scope == "device" || scope == "account" || scope == "device-owner" {
				alertState(t, p, r.ID, "firing")
			}
		})
	}
}

func TestMonitorAlertCRUDValidationAndSecretIsolation(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	c := alertChannel(t, p)
	stored, err := scanMonitorChannel(p.DB.QueryRow(`SELECT `+monitorChannelColumns+` FROM monitoring_channels WHERE id=?`, c.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !c.HasSecret || !strings.HasPrefix(stored.ciphertext, "v1:") || strings.Contains(stored.ciphertext, alertTestToken) {
		t.Fatal("secret not encrypted")
	}
	raw, _ := json.Marshal(c)
	if strings.Contains(string(raw), alertTestToken) || strings.Contains(string(raw), "ciphertext") || strings.Contains(string(raw), `"secret"`) {
		t.Fatalf("secret exposed: %s", raw)
	}
	plain, err := p.decryptMonitorSecret(stored)
	if err != nil || plain != alertTestToken {
		t.Fatalf("decrypt: %v", err)
	}
	before := stored.ciphertext
	updated, err := p.SaveMonitorChannel(1, c.ID, models.NotificationChannelInput{NotificationChannel: *c})
	if err != nil || !updated.HasSecret {
		t.Fatalf("preserve secret: %v", err)
	}
	stored, err = scanMonitorChannel(p.DB.QueryRow(`SELECT `+monitorChannelColumns+` FROM monitoring_channels WHERE id=?`, c.ID))
	if err != nil || stored.ciphertext != before {
		t.Fatal("empty input did not preserve ciphertext")
	}
	stored.AccountID++
	if _, err := p.decryptMonitorSecret(stored); err == nil {
		t.Fatal("cross-tenant ciphertext accepted")
	}
	stored.AccountID--
	stored.ID++
	if _, err := p.decryptMonitorSecret(stored); err == nil {
		t.Fatal("cross-channel ciphertext accepted")
	}
	stored.ID--
	stored.ciphertext += "X"
	if _, err := p.decryptMonitorSecret(stored); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	originalKey := p.Cfg.Auth.JWTSecret
	p.Cfg.Auth.JWTSecret = ""
	if _, err := p.SaveMonitorChannel(1, c.ID, models.NotificationChannelInput{NotificationChannel: *c}); !errors.Is(err, ErrMonitorInvalid) {
		t.Fatalf("missing key: %v", err)
	}
	p.Cfg.Auth.JWTSecret = originalKey
	alertExec(t, p, `INSERT INTO accounts(id,slug,name,status,created_at,updated_at) VALUES(2,'other','Other','active',?,?)`, now, now)
	r := alertRule(t, p, deviceID, now, "threshold", c.ID)
	if err := p.DeleteMonitorChannel(1, c.ID); !errors.Is(err, ErrMonitorConflict) {
		t.Fatalf("referenced channel: %v", err)
	}
	if _, err := p.SaveMonitorRule(2, 0, *r); !errors.Is(err, ErrMonitorInvalid) {
		t.Fatalf("foreign device: %v", err)
	}
	if _, err := p.SaveMonitorRule(2, r.ID, *r); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("foreign rule: %v", err)
	}
	if _, err := p.SaveMonitorChannel(2, c.ID, models.NotificationChannelInput{NotificationChannel: *c}); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("foreign channel: %v", err)
	}
	if err := p.DeleteMonitorRule(2, r.ID); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("foreign delete: %v", err)
	}
	if err := p.TestMonitorChannel(context.Background(), 2, c.ID); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("foreign test: %v", err)
	}
	alertSample(t, p, deviceID, now, 40)
	alertEvaluate(t, p, now)
	if got, err := p.ListMonitorEvents(2, 100); err != nil || len(got) != 0 {
		t.Fatalf("event isolation: %v %+v", err, got)
	}
	if got, err := p.ListMonitorDeliveries(2, 100); err != nil || len(got) != 0 {
		t.Fatalf("delivery isolation: %v %+v", err, got)
	}
	if got, err := p.ListMonitorRules(2); err != nil || len(got) != 0 {
		t.Fatal("rule isolation")
	}
	if got, err := p.ListMonitorChannels(2); err != nil || len(got) != 0 {
		t.Fatal("channel isolation")
	}

	for _, mutate := range []func(*models.AlertRule){
		func(r *models.AlertRule) { r.Kind = "script" }, func(r *models.AlertRule) { r.Metric = "" },
		func(r *models.AlertRule) { r.Operator = "gte" }, func(r *models.AlertRule) { r.Recovery = 40 },
		func(r *models.AlertRule) { r.Threshold = math.Inf(1) }, func(r *models.AlertRule) { r.Recovery = math.NaN() },
		func(r *models.AlertRule) { r.CooldownSeconds = 59 }, func(r *models.AlertRule) { r.MissingSeconds = -1 },
		func(r *models.AlertRule) { r.ForSeconds = 604801 }, func(r *models.AlertRule) { r.ChannelIDs = []int64{c.ID, c.ID} },
		func(r *models.AlertRule) { r.ChannelIDs = []int64{99999} }, func(r *models.AlertRule) { r.Name = strings.Repeat("x", 121) },
	} {
		input := *r
		mutate(&input)
		if _, err := p.SaveMonitorRule(1, 0, input); !errors.Is(err, ErrMonitorInvalid) {
			t.Fatalf("invalid rule accepted: %+v, %v", input, err)
		}
	}
	for _, input := range []models.NotificationChannelInput{
		{NotificationChannel: models.NotificationChannel{Name: "Bad", Kind: "webhook", Destination: "https://localhost"}},
		{NotificationChannel: models.NotificationChannel{Name: "Bad", Kind: "email", Destination: "a@example.com\r\nBcc: b@example.com"}},
		{NotificationChannel: models.NotificationChannel{Name: "Bad", Kind: "email", Destination: "a@example.com, b@example.com"}},
		{NotificationChannel: models.NotificationChannel{Name: "Bad", Kind: "telegram", Destination: "-1"}},
		{NotificationChannel: models.NotificationChannel{Name: "Bad", Kind: "telegram", Destination: "-1"}, Secret: "bad/token"},
	} {
		if _, err := p.SaveMonitorChannel(1, 0, input); !errors.Is(err, ErrMonitorInvalid) {
			t.Fatalf("bad channel accepted: %v", err)
		}
	}
	if err := p.DeleteMonitorRule(1, r.ID); err != nil {
		t.Fatal(err)
	}
	alertDeliveryCount(t, p, 0)
	if err := p.DeleteMonitorChannel(1, c.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorAlertLimitsAndConfigurationInvalidation(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	c := alertChannel(t, p)
	r := alertRule(t, p, deviceID, now, "threshold", c.ID)
	alertSample(t, p, deviceID, now, 40)
	alertEvaluate(t, p, now)
	if _, err := p.SaveMonitorRule(1, r.ID, *r); err != nil {
		t.Fatal(err)
	}
	if got := alertDeliveryCount(t, p, 1)[0]; got.Status != "failed" {
		t.Fatal("old rule delivery not invalidated")
	}
	alertEvaluate(t, p, now+1000)
	if _, err := p.SaveMonitorChannel(1, c.ID, models.NotificationChannelInput{NotificationChannel: *c}); err != nil {
		t.Fatal(err)
	}
	if got := alertDeliveryCount(t, p, 2)[0]; got.Status != "failed" {
		t.Fatal("old channel delivery not invalidated")
	}
	for i := 1; i < monitorMaxChannels; i++ {
		if _, err := p.SaveMonitorChannel(1, 0, models.NotificationChannelInput{NotificationChannel: models.NotificationChannel{Name: "Mail", Kind: "email", Destination: "alerts@example.test"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.SaveMonitorChannel(1, 0, models.NotificationChannelInput{NotificationChannel: models.NotificationChannel{Name: "Mail", Kind: "email", Destination: "alerts@example.test"}}); !errors.Is(err, ErrMonitorConflict) {
		t.Fatalf("channel quota: %v", err)
	}
	for i := 1; i < monitorMaxRules; i++ {
		if _, err := p.SaveMonitorRule(1, 0, *r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.SaveMonitorRule(1, 0, *r); !errors.Is(err, ErrMonitorConflict) {
		t.Fatalf("rule quota: %v", err)
	}
}

func TestMonitorAlertAtomicTransitionAndOutboxUniqueness(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	c := alertChannel(t, p)
	r := alertRule(t, p, deviceID, now, "threshold", c.ID)
	alertSample(t, p, deviceID, now, 40)
	alertExec(t, p, `CREATE TRIGGER fail_alert_outbox BEFORE INSERT ON monitoring_deliveries BEGIN SELECT RAISE(ABORT,'injected outbox failure'); END`)
	if err := p.monitorEvaluate(context.Background(), now); err == nil {
		t.Fatal("injected outbox failure ignored")
	}
	alertState(t, p, r.ID, "normal")
	if events, err := p.ListMonitorEvents(1, 100); err != nil || len(events) != 0 {
		t.Fatal("event survived rolled back outbox")
	}
	alertExec(t, p, `DROP TRIGGER fail_alert_outbox`)
	alertEvaluate(t, p, now)
	d := alertDeliveryCount(t, p, 1)[0]
	if _, err := p.DB.Exec(`INSERT INTO monitoring_deliveries(account_id,event_id,channel_id,next_attempt,created_at,updated_at) VALUES(1,?,?,?,?,?)`, d.EventID, c.ID, now, now, now); err == nil {
		t.Fatal("duplicate delivery accepted")
	}
}

func TestMonitorAlertDurableOutboxRetryClaimAndFencing(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	c := alertChannel(t, p)
	alertRule(t, p, deviceID, now, "threshold", c.ID)
	alertSample(t, p, deviceID, now, 40)
	alertEvaluate(t, p, now)
	ctx := context.Background()
	var wg sync.WaitGroup
	claims := make(chan *monitorClaim, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := p.monitorClaimDelivery(ctx, now)
			if err != nil {
				errs <- err
			}
			if claim != nil {
				claims <- claim
			}
		}()
	}
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var first *monitorClaim
	for c := range claims {
		if first != nil {
			t.Fatal("delivery claimed twice")
		}
		first = c
	}
	if first == nil {
		t.Fatal("no claim")
	}
	if c, err := p.monitorClaimDelivery(ctx, now+monitorLeaseMS-1); c != nil || err != nil {
		t.Fatal("active lease stolen")
	}
	// Another process/restart can reclaim only after expiration.
	restarted := New(p.DB, p.Cfg, p.Log)
	second, err := restarted.monitorClaimDelivery(ctx, now+monitorLeaseMS)
	if err != nil || second == nil || second.token == first.token || second.attempts != 2 {
		t.Fatalf("reclaim: %+v %v", second, err)
	}
	if err := p.monitorFinishDelivery(first, now+monitorLeaseMS, nil); err != nil {
		t.Fatal(err)
	}
	if got := alertDeliveryCount(t, p, 1)[0]; got.Status != "sending" {
		t.Fatal("late completion overrode new lease")
	}
	if err := restarted.monitorFinishDelivery(second, now+monitorLeaseMS, fmt.Errorf("unsafe transport error: %s", alertTestToken)); err != nil {
		t.Fatal(err)
	}
	got := alertDeliveryCount(t, p, 1)[0]
	if got.Status != "pending" || got.NextAttempt != now+120000 || strings.Contains(got.LastError, alertTestToken) {
		t.Fatalf("retry: %+v", got)
	}
	if c, err := p.monitorClaimDelivery(ctx, got.NextAttempt-1); err != nil || c != nil {
		t.Fatal("backoff ignored")
	}
	for attempt := 3; attempt <= monitorMaxAttempts; attempt++ {
		claim, err := p.monitorClaimDelivery(ctx, got.NextAttempt)
		if err != nil || claim == nil || claim.attempts != attempt {
			t.Fatalf("claim %d: %v %+v", attempt, err, claim)
		}
		if err := p.monitorFinishDelivery(claim, got.NextAttempt, &monitorSendError{message: "Telegram notification rejected", retryAfter: time.Minute * 3}); err != nil {
			t.Fatal(err)
		}
		got = alertDeliveryCount(t, p, 1)[0]
	}
	if got.Status != "failed" {
		t.Fatalf("unbounded retry: %+v", got)
	}
	if claim, err := p.monitorClaimDelivery(ctx, got.NextAttempt+86400000); err != nil || claim != nil {
		t.Fatal("failed delivery reclaimed")
	}
}

func TestMonitorAlertDispatchSuccessAndCancellation(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	c := alertChannel(t, p)
	alertRule(t, p, deviceID, now, "threshold", c.ID)
	alertSample(t, p, deviceID, now, 40)
	alertEvaluate(t, p, now)
	calls := 0
	worked, err := p.monitorDispatchOnce(context.Background(), now, func(ctx context.Context, channel monitorStoredChannel, name, state, message string) error {
		calls++
		if channel.ID != c.ID || state != "firing" || !strings.Contains(message, "temperature") {
			t.Fatal("wrong notification")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("send lacks deadline")
		}
		return nil
	})
	if err != nil || !worked || calls != 1 {
		t.Fatalf("send %v %v %d", worked, err, calls)
	}
	if got := alertDeliveryCount(t, p, 1)[0]; got.Status != "sent" || got.Attempts != 1 {
		t.Fatalf("sent: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.TestMonitorChannel(ctx, 1, c.ID); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := p.monitorEvaluate(ctx, now); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := p.monitorClaimDelivery(ctx, now); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { p.MonitoringLoop(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop ignored cancellation")
	}
}

func TestMonitorAlertReceiveTimeAndReplayedRecovery(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	r := alertRule(t, p, deviceID, now, "threshold")
	alertSample(t, p, deviceID, now, 40)
	alertEvaluate(t, p, now)
	alertState(t, p, r.ID, "firing")
	// A newly received but old measurement is not evidence of recovery.
	alertExec(t, p, `INSERT INTO telemetry_samples(device_pk,account_id,key,value_num,ts,ingested_at) VALUES(?,1,'temperature',20,?,?)`, deviceID, now-300000, now+1000)
	alertEvaluate(t, p, now+1000)
	alertState(t, p, r.ID, "firing")
	// Neither is a device clock far in the future.
	alertExec(t, p, `INSERT INTO telemetry_samples(device_pk,account_id,key,value_num,ts,ingested_at) VALUES(?,1,'temperature',20,?,?)`, deviceID, now+86400000, now+2000)
	alertEvaluate(t, p, now+2000)
	alertState(t, p, r.ID, "firing")
	noData := alertRule(t, p, deviceID, now, "no_data")
	alertEvaluate(t, p, now+121000)
	alertState(t, p, noData.ID, "normal") // receive time, not the future device clock
	alertEvaluate(t, p, now+122000)
	alertState(t, p, noData.ID, "firing")
}

func TestMonitorAlertConcurrentEvaluatorsAndAbandonedFinalLease(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	c := alertChannel(t, p)
	alertRule(t, p, deviceID, now, "threshold", c.ID)
	alertSample(t, p, deviceID, now, 40)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- p.monitorEvaluate(context.Background(), now) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	d := alertDeliveryCount(t, p, 1)[0]
	alertExec(t, p, `UPDATE monitoring_deliveries SET status='sending', attempts=?, lease_until=?, lease_token='abandoned' WHERE id=?`, monitorMaxAttempts, now+monitorLeaseMS, d.ID)
	if claim, err := p.monitorClaimDelivery(context.Background(), now+monitorLeaseMS-1); err != nil || claim != nil {
		t.Fatal("final lease stolen")
	}
	if got := alertDeliveryCount(t, p, 1)[0]; got.Status != "sending" {
		t.Fatal("live attempt marked failed")
	}
	if claim, err := p.monitorClaimDelivery(context.Background(), now+monitorLeaseMS); err != nil || claim != nil {
		t.Fatal("attempt limit ignored after crash")
	}
	if got := alertDeliveryCount(t, p, 1)[0]; got.Status != "failed" {
		t.Fatal("abandoned final attempt never failed")
	}
}

func TestMonitorAlertRunningLoopAndInFlightCancellation(t *testing.T) {
	p, deviceID, now := alertFixture(t)
	alertRule(t, p, deviceID, now, "offline") // no notification channels: no network
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { p.MonitoringLoop(ctx); close(done) }()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		events, err := p.ListMonitorEvents(1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 1 {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("server loop did not evaluate offline rule")
		case <-poll.C:
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("running loop did not stop")
	}

	c := alertChannel(t, p)
	alertRule(t, p, deviceID, now, "threshold", c.ID)
	alertSample(t, p, deviceID, now, 40)
	alertEvaluate(t, p, now)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	completed := make(chan error, 1)
	go func() {
		_, err := p.monitorDispatchOnce(ctx, now, func(ctx context.Context, _ monitorStoredChannel, _, _, _ string) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		})
		completed <- err
	}()
	<-entered
	cancel()
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("delivery ignored cancellation")
	}
	if got := alertDeliveryCount(t, p, 1)[0]; got.Status != "pending" {
		t.Fatal("canceled delivery lost instead of retried")
	}
}

type alertRoundTripFunc func(*http.Request) (*http.Response, error)

func (f alertRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMonitorTelegramLocalHTTPTransport(t *testing.T) {
	message := "Alert \"quoted\"\n<script> & unicode: температура"
	for _, tc := range []struct {
		name              string
		status            int
		body              string
		retry             string
		wantOK, permanent bool
		delay             time.Duration
	}{
		{"ok", 200, `{"ok":true}`, "", true, false, 0},
		{"api-error", 200, `{"ok":false,"error_code":403,"description":"` + alertTestToken + `"}`, "", false, true, 0},
		{"http-error", 503, `{"ok":true}`, "", false, false, 0},
		{"rate-limit", 429, `{"ok":false,"parameters":{"retry_after":120}}`, "180", false, false, 180 * time.Second},
		{"bad-json", 200, `not json`, "", false, false, 0},
		{"oversized", 200, `{"ok":true,"padding":"` + strings.Repeat("x", 70*1024) + `"}`, "", false, false, 0},
		{"redirect", 302, ``, "", false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("invalid request")
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload["text"] != message || payload["chat_id"] != "-123" || payload["parse_mode"] != nil {
					t.Error("JSON escaped incorrectly or HTML enabled")
				}
				w.Header().Set("Retry-After", tc.retry)
				w.Header().Set("Location", "https://never-follow.invalid/leak")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			local, _ := url.Parse(server.URL)
			transport := alertRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme != "https" || r.URL.Host != "api.telegram.org" || r.URL.Path != "/bot"+alertTestToken+"/sendMessage" {
					t.Fatal("non-fixed Telegram endpoint")
				}
				clone := r.Clone(r.Context())
				u := *r.URL
				clone.URL = &u
				clone.URL.Scheme = local.Scheme
				clone.URL.Host = local.Host
				return server.Client().Transport.RoundTrip(clone)
			})
			err := sendMonitorTelegram(context.Background(), transport, alertTestToken, "-123", message)
			if tc.wantOK {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var safe *monitorSendError
				if !errors.As(err, &safe) || safe.permanent != tc.permanent || safe.retryAfter != tc.delay {
					t.Fatalf("error=%+v", err)
				}
				if strings.Contains(err.Error(), alertTestToken) {
					t.Fatal("token leaked")
				}
			}
			if requests.Load() != 1 {
				t.Fatal("redirect followed or unexpected request")
			}
		})
	}
	transport := alertRoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("URL %s", r.URL) })
	if err := sendMonitorTelegram(context.Background(), transport, alertTestToken, "-1", "test"); err == nil || strings.Contains(err.Error(), alertTestToken) {
		t.Fatal("transport token leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	transport = alertRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	done := make(chan error, 1)
	go func() { done <- sendMonitorTelegram(ctx, transport, alertTestToken, "-1", "test") }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP ignored cancel")
	}
	date := time.Now().UTC().Truncate(time.Second)
	if got := monitorRetryAfter(date.Add(time.Minute).Format(http.TimeFormat), date); got != time.Minute {
		t.Fatal(got)
	}
	if got := monitorRetryAfter("9223372036854775807", date); got != 24*time.Hour {
		t.Fatal("retry delay overflow")
	}
}
