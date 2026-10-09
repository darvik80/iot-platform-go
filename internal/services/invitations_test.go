package services

import (
	"net/url"
	"strings"
	"testing"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
)

func inviteTokenFromURL(t *testing.T, u string) string {
	t.Helper()
	i := strings.LastIndexByte(u, '/')
	tok, err := url.PathUnescape(u[i+1:])
	if err != nil {
		t.Fatalf("unescape invite url: %v", err)
	}
	return tok
}

func newAccountAdmin(t *testing.T, p *Platform, email, login string) int64 {
	t.Helper()
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatal(err)
	}
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)`, email, hash, "admin", login, login, 1, now, now)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := p.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at)
		VALUES(?,1,'admin','active',?)`, id, now); err != nil {
		t.Fatal(err)
	}
	return id
}

func newPlainUser(t *testing.T, p *Platform, email, login string) int64 {
	t.Helper()
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatal(err)
	}
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)`, email, hash, "viewer", login, login, 1, now, now)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestRegisterInvitedUser covers the invite-signup happy path: the invited
// person arrives with only a link and sets their own password. Public
// registration stays disabled — an invitation is an explicit admin grant.
func TestRegisterInvitedUser(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if p.Cfg.Register.Enabled {
		t.Fatal("test assumes public registration disabled")
	}
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")

	_, inviteURL, err := p.CreateInvitation(1, inviter, "Newbie@Example.com", "operator", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	u, jwt, err := p.RegisterInvitedUser(token, InviteRegisterInput{
		Login: "Newbie", Password: "passw0rd1", DisplayName: "New Bee",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "newbie@example.com" {
		t.Errorf("email=%q want invitation email, lowercased", u.Email)
	}
	if u.Login != "newbie" || u.DisplayName != "New Bee" {
		t.Errorf("login=%q display=%q", u.Login, u.DisplayName)
	}
	if !u.EmailVerified {
		t.Error("invited user must be email_verified (link possession)")
	}
	if u.Role != "operator" {
		t.Errorf("role=%q want invitation role", u.Role)
	}
	if jwt == "" {
		t.Error("empty session token")
	}

	m, err := p.GetMembership(u.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Role != "operator" || m.Status != "active" {
		t.Errorf("membership=%+v", m)
	}
	if m.InvitedBy == nil || *m.InvitedBy != inviter {
		t.Errorf("invited_by=%v want %d", m.InvitedBy, inviter)
	}

	var status string
	if err := p.DB.QueryRow(`SELECT status FROM account_invitations WHERE email=?`, u.Email).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "accepted" {
		t.Errorf("invitation status=%q", status)
	}

	// the chosen password actually signs the new user in
	if _, _, err := p.Login(u.Email, "passw0rd1"); err != nil {
		t.Errorf("login with chosen password: %v", err)
	}

	// the token is single-use
	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "other", Password: "passw0rd1"}); err == nil {
		t.Error("second register with same token must fail")
	}
}

func TestRegisterInvitedUserEmailTaken(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	newPlainUser(t, p, "taken@example.com", "taken")

	_, inviteURL, err := p.CreateInvitation(1, inviter, "taken@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.RegisterInvitedUser(inviteTokenFromURL(t, inviteURL), InviteRegisterInput{Login: "dup", Password: "passw0rd1"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v want already-exists", err)
	}
}

func TestRegisterInvitedUserValidation(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "new@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	for name, in := range map[string]InviteRegisterInput{
		"bad login":     {Login: "1abc", Password: "passw0rd1"},
		"short login":   {Login: "ab", Password: "passw0rd1"},
		"short pass":    {Login: "newbie", Password: "p1"},
		"no digit":      {Login: "newbie", Password: "password"},
		"no letter":     {Login: "newbie", Password: "12345678"},
	} {
		if _, _, err := p.RegisterInvitedUser(token, in); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	// the failed attempts must not have burned the invitation
	var status string
	if err := p.DB.QueryRow(`SELECT status FROM account_invitations WHERE email=?`, "new@example.com").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Errorf("status=%q after failed attempts", status)
	}
	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "newbie", Password: "passw0rd1"}); err != nil {
		t.Errorf("register after failed attempts: %v", err)
	}

	if _, _, err := p.RegisterInvitedUser("no-such-token", InviteRegisterInput{Login: "x", Password: "passw0rd1"}); err == nil {
		t.Error("unknown token must fail")
	}
}

func TestRegisterInvitedUserExpired(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	inv, inviteURL, err := p.CreateInvitation(1, inviter, "late@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DB.Exec(`UPDATE account_invitations SET expires_at=? WHERE id=?`, db.NowMS()-1, inv.ID); err != nil {
		t.Fatal(err)
	}

	_, _, err = p.RegisterInvitedUser(inviteTokenFromURL(t, inviteURL), InviteRegisterInput{Login: "late", Password: "passw0rd1"})
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("err=%v want expired", err)
	}
	var status string
	if err := p.DB.QueryRow(`SELECT status FROM account_invitations WHERE id=?`, inv.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "expired" {
		t.Errorf("status=%q", status)
	}
}

func TestAcceptInvitationEmailGuard(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	other := newPlainUser(t, p, "someone-else@example.com", "elsewhere")
	rightUser := newPlainUser(t, p, "right-person@example.com", "right")

	_, inviteURL, err := p.CreateInvitation(1, inviter, "right-person@example.com", "operator", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	if _, err := p.AcceptInvitation(token, other); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err=%v want email mismatch", err)
	}

	m, err := p.AcceptInvitation(token, rightUser)
	if err != nil {
		t.Fatal(err)
	}
	if m.Role != "operator" || m.AccountID != 1 {
		t.Errorf("membership=%+v", m)
	}
	if m.InvitedBy == nil || *m.InvitedBy != inviter {
		t.Errorf("invited_by=%v want %d (accept path must record the inviter)", m.InvitedBy, inviter)
	}
}
