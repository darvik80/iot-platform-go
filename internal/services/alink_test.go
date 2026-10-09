package services

import (
	"testing"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/protocol"
)

func TestCreateDeviceAlinkStyle(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if _, err := p.CreateProduct(1, ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatal(err)
	}
	d, creds, err := p.CreateDevice(1, DeviceInput{
		ProductID: "home", DeviceID: "esp1",
		TopicStyle: protocol.StyleAlink, AlinkSecret: "s3cret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.TopicStyle != protocol.StyleAlink {
		t.Errorf("topic_style=%q", d.TopicStyle)
	}
	if creds.Username != "esp1&home" || creds.ClientID != "esp1&home" {
		t.Errorf("username/client_id=%q/%q", creds.Username, creds.ClientID)
	}
	if creds.DeviceSecret != "s3cret" {
		t.Errorf("device_secret=%q", creds.DeviceSecret)
	}
	if creds.TopicStyle != protocol.StyleAlink {
		t.Errorf("creds topic_style=%q", creds.TopicStyle)
	}
	if creds.Provision == nil || creds.Provision.Cogitor.Token != "s3cret" || creds.Provision.Cogitor.TopicShort {
		t.Errorf("provision=%+v", creds.Provision)
	}

	var style, secret, hash string
	if err := p.DB.QueryRow(`SELECT topic_style, COALESCE(alink_secret,''), COALESCE(access_token_hash,'') FROM devices WHERE id=?`, d.ID).
		Scan(&style, &secret, &hash); err != nil {
		t.Fatal(err)
	}
	if style != "alink" || secret != "s3cret" || hash != "" {
		t.Errorf("row style=%q secret=%q hash=%q", style, secret, hash)
	}
	// no token credential row exists for an alink device
	var rows int
	if err := p.DB.QueryRow(`SELECT COUNT(1) FROM device_credentials WHERE device_pk=?`, d.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("device_credentials rows=%d want 0", rows)
	}
	if r := p.ResolveByClientID("esp1&home"); r == nil || r.ID != d.ID {
		t.Errorf("ResolveByClientID(esp1&home)=%v", r)
	}
	if p.ResolveByToken("s3cret") != nil {
		t.Error("alink secret must not resolve as an access token")
	}
}

func TestCreateDeviceAlinkGeneratedSecret(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if _, err := p.CreateProduct(1, ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatal(err)
	}
	_, creds, err := p.CreateDevice(1, DeviceInput{ProductID: "home", DeviceID: "esp1", TopicStyle: protocol.StyleAlink})
	if err != nil {
		t.Fatal(err)
	}
	if creds.DeviceSecret == "" {
		t.Fatal("no secret generated")
	}
	user := protocol.AlinkUsername("home", "esp1")
	cid := user + "|timestamp=" + protocol.AlinkDefaultTimestamp + ",ext=3|"
	sign := protocol.AlinkSign(creds.DeviceSecret, "home", "esp1", protocol.AlinkDefaultTimestamp)
	if !protocol.AlinkVerifyAuth(sign, creds.DeviceSecret, "home", "esp1", cid) {
		t.Error("generated secret does not verify")
	}
	var secret string
	if err := p.DB.QueryRow(`SELECT COALESCE(alink_secret,'') FROM devices WHERE device_id=?`, "esp1").Scan(&secret); err != nil {
		t.Fatal(err)
	}
	if secret != creds.DeviceSecret {
		t.Errorf("stored secret %q != returned %q", secret, creds.DeviceSecret)
	}
}

func TestCreateDeviceAlinkFromProductDefault(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if _, err := p.CreateProduct(1, ProductInput{ProductID: "home", Name: "Home", TopicStyleDefault: protocol.StyleAlink}); err != nil {
		t.Fatal(err)
	}
	d, creds, err := p.CreateDevice(1, DeviceInput{ProductID: "home", DeviceID: "esp1"})
	if err != nil {
		t.Fatal(err)
	}
	if d.TopicStyle != protocol.StyleAlink {
		t.Errorf("topic_style=%q want product default alink", d.TopicStyle)
	}
	if creds.DeviceSecret == "" {
		t.Error("no secret generated from product default")
	}
}

func TestCreateDeviceInvalidStyle(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if _, err := p.CreateProduct(1, ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.CreateDevice(1, DeviceInput{ProductID: "home", DeviceID: "esp1", TopicStyle: "bogus"}); err == nil {
		t.Error("invalid topic_style accepted")
	}
}

func TestRotateAlinkSecret(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if _, err := p.CreateProduct(1, ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatal(err)
	}
	d, _, err := p.CreateDevice(1, DeviceInput{
		ProductID: "home", DeviceID: "esp1",
		TopicStyle: protocol.StyleAlink, AlinkSecret: "old-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	creds, err := p.RotateDeviceToken(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if creds.DeviceSecret == "" || creds.DeviceSecret == "old-secret" {
		t.Errorf("rotated secret=%q", creds.DeviceSecret)
	}
	if creds.Username != "esp1&home" {
		t.Errorf("username=%q (must survive rotation)", creds.Username)
	}
	if creds.TopicStyle != protocol.StyleAlink {
		t.Errorf("creds topic_style=%q", creds.TopicStyle)
	}
	var secret string
	if err := p.DB.QueryRow(`SELECT COALESCE(alink_secret,'') FROM devices WHERE id=?`, d.ID).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	if secret != creds.DeviceSecret {
		t.Errorf("stored secret %q != returned %q", secret, creds.DeviceSecret)
	}
	if r := p.ResolveByClientID("esp1&home"); r == nil || r.ID != d.ID {
		t.Errorf("ResolveByClientID after rotate=%v", r)
	}
}

func TestUpdateDeviceStyleSwitch(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if _, err := p.CreateProduct(1, ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatal(err)
	}
	d, creds, err := p.CreateDevice(1, DeviceInput{ProductID: "home", DeviceID: "esp1"})
	if err != nil {
		t.Fatal(err)
	}
	oldToken := creds.AccessToken

	// switching to alink without a secret is an explicit error
	if _, err := p.UpdateDevice(d.ID, DeviceInput{TopicStyle: protocol.StyleAlink}); err == nil {
		t.Error("style switch to alink without secret accepted")
	}

	// switching with a secret keeps the device working under alink auth
	d2, err := p.UpdateDevice(d.ID, DeviceInput{TopicStyle: protocol.StyleAlink, AlinkSecret: "switched"})
	if err != nil {
		t.Fatal(err)
	}
	if d2.TopicStyle != protocol.StyleAlink {
		t.Fatalf("topic_style=%q", d2.TopicStyle)
	}
	var style, secret, hash string
	if err := p.DB.QueryRow(`SELECT topic_style, COALESCE(alink_secret,''), COALESCE(access_token_hash,'') FROM devices WHERE id=?`, d.ID).
		Scan(&style, &secret, &hash); err != nil {
		t.Fatal(err)
	}
	if style != "alink" || secret != "switched" || hash != "" {
		t.Errorf("row style=%q secret=%q hash=%q", style, secret, hash)
	}
	var active int
	if err := p.DB.QueryRow(`SELECT COUNT(1) FROM device_credentials WHERE device_pk=? AND revoked_at IS NULL`, d.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Errorf("active device_credentials=%d want 0 (revoked on switch)", active)
	}
	if p.ResolveByToken(oldToken) != nil {
		t.Error("old token still resolves after switch to alink")
	}

	// updating the secret without changing style
	if _, err := p.UpdateDevice(d.ID, DeviceInput{AlinkSecret: "renewed"}); err != nil {
		t.Fatal(err)
	}
	if err := p.DB.QueryRow(`SELECT COALESCE(alink_secret,'') FROM devices WHERE id=?`, d.ID).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	if secret != "renewed" {
		t.Errorf("secret=%q want renewed", secret)
	}

	// switching back to a token style drops the secret entirely
	d3, err := p.UpdateDevice(d.ID, DeviceInput{TopicStyle: protocol.StyleShort})
	if err != nil {
		t.Fatal(err)
	}
	if d3.TopicStyle != protocol.StyleShort {
		t.Fatalf("topic_style=%q", d3.TopicStyle)
	}
	if err := p.DB.QueryRow(`SELECT topic_style, COALESCE(alink_secret,'') FROM devices WHERE id=?`, d.ID).
		Scan(&style, &secret); err != nil {
		t.Fatal(err)
	}
	if style != "short" || secret != "" {
		t.Errorf("row style=%q secret=%q", style, secret)
	}
	if p.ResolveByClientID("esp1&home") != nil {
		t.Error("alink username still resolves after switch away from alink")
	}
}
