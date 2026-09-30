package services

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
)

func TestValidEmail(t *testing.T) {
	for s, want := range map[string]bool{
		"a@b.co":           true,
		"user.name@ex.org": true,
		"no-at.example":    false,
		"a@b":              false,
		"a@@b.co":          false,
		"@b.co":            false,
		"a@.b.co":          false,
		"a@b.co.":          false,
		"a b@c.co":         false,
		"<x>@c.co":         false,
	} {
		if got := validEmail(s); got != want {
			t.Errorf("validEmail(%q)=%v want %v", s, got, want)
		}
	}
}

func TestBootstrapSetupRecordsPlatformAdmin(t *testing.T) {
	database, err := db.Open("sqlite", filepath.Join(t.TempDir(), "setup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	var cfg config.Config
	cfg.Admin = config.AdminConfig{Login: "root", Email: "root@example.com", Password: "password123"}
	cfg.Bootstrap = config.BootstrapConfig{AccountSlug: "main", AccountName: "Main"}
	if err := bootstrapSetup(database, cfg); err != nil {
		t.Fatal(err)
	}
	p := New(database, cfg, nil)
	var adminID int64
	if err := database.QueryRow(`SELECT id FROM users WHERE login='root'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if !p.IsPlatformAdmin(adminID) {
		t.Errorf("wizard admin %d is not the platform admin (stored %d)", adminID, p.PlatformAdminID())
	}
}

func TestEnsureAdminAssignsPlatformAdminOnLegacyDB(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	p.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	now := db.NowMS()
	for _, login := range []string{"first", "second"} {
		if _, err := p.DB.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
			VALUES(?, '', 'admin', ?, ?, 1, ?, ?)`, login+"@example.com", login, login, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.EnsureAdmin(); err != nil {
		t.Fatal(err)
	}
	first, _ := p.FindUserByLoginOrEmail("first")
	second, _ := p.FindUserByLoginOrEmail("second")
	if !p.IsPlatformAdmin(first.ID) || p.IsPlatformAdmin(second.ID) {
		t.Errorf("platform admin = %d, want the oldest admin %d", p.PlatformAdminID(), first.ID)
	}
}

func TestNotifyFirmwareTargetsOnlyOwnProduct(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO accounts(slug, name, status, created_at, updated_at) VALUES('other', 'Other', 'active', ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := res.LastInsertId()
	for _, acc := range []int64{1, other} {
		if _, err := p.CreateProduct(acc, ProductInput{ProductID: "lamp", Name: "Lamp"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := p.CreateDevice(1, DeviceInput{ProductID: "lamp", DeviceID: "mine"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.CreateDevice(other, DeviceInput{ProductID: "lamp", DeviceID: "theirs"}); err != nil {
		t.Fatal(err)
	}
	fw, err := p.RegisterFirmware(1, FirmwareInput{ProductID: "lamp", Version: "2.0.0", URL: "http://example.com/fw.bin"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.NotifyFirmware(fw.ID, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range out.Errors {
		t.Errorf("notify touched a foreign device: %s", e)
	}
	if out.Started+out.Failed+out.Skipped != 1 {
		t.Errorf("notify considered %d devices, want 1: %+v", out.Started+out.Failed+out.Skipped, out)
	}
	if !p.FirmwareInAccount(1, fw.ID) || p.FirmwareInAccount(other, fw.ID) {
		t.Error("FirmwareInAccount does not match the owning account")
	}
}
