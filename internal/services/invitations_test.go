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
		"bad login":   {Login: "1abc", Password: "passw0rd1"},
		"short login": {Login: "ab", Password: "passw0rd1"},
		"short pass":  {Login: "newbie", Password: "p1"},
		"no digit":    {Login: "newbie", Password: "password"},
		"no letter":   {Login: "newbie", Password: "12345678"},
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

// ── GetInvitation ────────────────────────────────────────────────────────────

func TestGetInvitationPending(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "guest@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	info, err := p.GetInvitation(inviteTokenFromURL(t, inviteURL))
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "pending" {
		t.Errorf("status=%q want pending", info.Status)
	}
	if info.Email != "guest@example.com" {
		t.Errorf("email=%q", info.Email)
	}
	if info.Role != "viewer" {
		t.Errorf("role=%q", info.Role)
	}
	if info.AccountName == "" {
		t.Error("account name empty")
	}
}

func TestGetInvitationNotFound(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	_, err := p.GetInvitation("no-such-token")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v want not-found", err)
	}
}

func TestGetInvitationEmptyToken(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	_, err := p.GetInvitation("")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err=%v want missing-token", err)
	}
}

func TestGetInvitationAccepted(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	newPlainUser(t, p, "done@example.com", "done")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "done@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	// accept it
	doneUser, _ := p.FindUserByLoginOrEmail("done@example.com")
	if _, err := p.AcceptInvitation(token, doneUser.ID); err != nil {
		t.Fatal(err)
	}

	info, err := p.GetInvitation(token)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "accepted" {
		t.Errorf("status=%q want accepted", info.Status)
	}
}

func TestGetInvitationRevoked(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "revoked@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	// find invitation id
	var invID int64
	if err := p.DB.QueryRow(`SELECT id FROM account_invitations WHERE token_hash=?`, hashInviteToken(token)).Scan(&invID); err != nil {
		t.Fatal(err)
	}
	if err := p.RevokeInvitation(1, invID); err != nil {
		t.Fatal(err)
	}

	info, err := p.GetInvitation(token)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "revoked" {
		t.Errorf("status=%q want revoked", info.Status)
	}
}

func TestGetInvitationExpiredOnRead(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "late@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	// backdate expiry
	if _, err := p.DB.Exec(`UPDATE account_invitations SET expires_at=? WHERE token_hash=?`, db.NowMS()-1, hashInviteToken(token)); err != nil {
		t.Fatal(err)
	}

	info, err := p.GetInvitation(token)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "expired" {
		t.Errorf("status=%q want expired", info.Status)
	}
}

// ─── AcceptInvitation edge cases ─────────────────────────────────────────────

func TestAcceptInvitationAlreadyAccepted(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	user := newPlainUser(t, p, "acc@example.com", "acc")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "acc@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	if _, err := p.AcceptInvitation(token, user); err != nil {
		t.Fatal(err)
	}
	// second accept must fail
	if _, err := p.AcceptInvitation(token, user); err == nil || !strings.Contains(err.Error(), "no longer pending") {
		t.Fatalf("err=%v want no-longer-pending", err)
	}
}

func TestAcceptInvitationRevoked(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	user := newPlainUser(t, p, "rev@example.com", "rev")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "rev@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	var invID int64
	_ = p.DB.QueryRow(`SELECT id FROM account_invitations WHERE token_hash=?`, hashInviteToken(token)).Scan(&invID)
	if err := p.RevokeInvitation(1, invID); err != nil {
		t.Fatal(err)
	}

	if _, err := p.AcceptInvitation(token, user); err == nil || !strings.Contains(err.Error(), "no longer pending") {
		t.Fatalf("err=%v want no-longer-pending", err)
	}
}

func TestAcceptInvitationExpired(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	user := newPlainUser(t, p, "exp@example.com", "exp")
	inv, inviteURL, err := p.CreateInvitation(1, inviter, "exp@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	// backdate expiry
	if _, err := p.DB.Exec(`UPDATE account_invitations SET expires_at=? WHERE id=?`, db.NowMS()-1, inv.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := p.AcceptInvitation(token, user); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("err=%v want expired", err)
	}
	// DB status should be updated to expired
	var status string
	_ = p.DB.QueryRow(`SELECT status FROM account_invitations WHERE id=?`, inv.ID).Scan(&status)
	if status != "expired" {
		t.Errorf("db status=%q want expired", status)
	}
}

func TestAcceptInvitationAlreadyMember(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	// create invitation for email X
	_, inviteURL, err := p.CreateInvitation(1, inviter, "member@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	// create user with email X and add as member BEFORE accepting
	user := newPlainUser(t, p, "member@example.com", "member")
	now := db.NowMS()
	if _, err := p.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at) VALUES(?,1,'viewer','active',?)`, user, now); err != nil {
		t.Fatal(err)
	}

	if _, err := p.AcceptInvitation(token, user); err == nil || !strings.Contains(err.Error(), "already a member") {
		t.Fatalf("err=%v want already-a-member", err)
	}
}

func TestAcceptInvitationUnknownToken(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if _, err := p.AcceptInvitation("bogus", 1); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v want not-found", err)
	}
}

func TestAcceptInvitationEmptyToken(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	if _, err := p.AcceptInvitation("", 1); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err=%v want missing", err)
	}
}

// ─── RegisterInvitedUser edge cases ──────────────────────────────────────────

func TestRegisterInvitedUserAlreadyAccepted(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "first@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "first", Password: "passw0rd1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "second", Password: "passw0rd1"}); err == nil || !strings.Contains(err.Error(), "no longer pending") {
		t.Fatalf("err=%v want no-longer-pending", err)
	}
}

func TestRegisterInvitedUserRevoked(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "x@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	var invID int64
	_ = p.DB.QueryRow(`SELECT id FROM account_invitations WHERE token_hash=?`, hashInviteToken(token)).Scan(&invID)
	if err := p.RevokeInvitation(1, invID); err != nil {
		t.Fatal(err)
	}

	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "xuser", Password: "passw0rd1"}); err == nil || !strings.Contains(err.Error(), "no longer pending") {
		t.Fatalf("err=%v want no-longer-pending", err)
	}
}

func TestRegisterInvitedUserLoginTaken(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	newPlainUser(t, p, "other@example.com", "takenlogin")

	_, inviteURL, err := p.CreateInvitation(1, inviter, "fresh@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "takenlogin", Password: "passw0rd1"}); err == nil || !strings.Contains(err.Error(), "login already taken") {
		t.Fatalf("err=%v want login-taken", err)
	}
	// invitation must still be usable
	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "fresh", Password: "passw0rd1"}); err != nil {
		t.Errorf("register with different login: %v", err)
	}
}

func TestRegisterInvitedUserEmailTakenDifferentAccount(t *testing.T) {
	// User exists in a DIFFERENT account — register must still fail because
	// the email is globally unique. This is the "user exists but not under
	// this account" scenario.
	p := newTestPlatform(t, config.Config{})

	// create second account
	now := db.NowMS()
	if _, err := p.DB.Exec(`INSERT INTO accounts(id, slug, name, status, created_at, updated_at) VALUES(2, 'other', 'Other Account', 'active', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}

	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	// make inviter also admin of account 2
	if _, err := p.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at) VALUES(?,2,'admin','active',?)`, inviter, now); err != nil {
		t.Fatal(err)
	}
	// user exists globally with this email (in account 1 as admin)
	existingUser := newAccountAdmin(t, p, "conflict@example.com", "conflict")
	_ = existingUser // just need the user row to exist

	_, inviteURL, err := p.CreateInvitation(2, inviter, "conflict@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "newbie", Password: "passw0rd1"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v want already-exists (email is global)", err)
	}
}

func TestRegisterInvitedUserReactivateDisabled(t *testing.T) {
	// User exists but is disabled. Invitation register should reactivate them
	// instead of blocking with "already exists".
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")

	// create a disabled user with the invited email
	hash, _ := auth.HashPassword("oldpass1")
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, disabled, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, "disabled@example.com", hash, "viewer", "oldlogin", "Old Name", 1, 1, now, now)
	if err != nil {
		t.Fatal(err)
	}
	disabledUserID, _ := res.LastInsertId()

	_, inviteURL, err := p.CreateInvitation(1, inviter, "disabled@example.com", "operator", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	u, jwt, err := p.RegisterInvitedUser(token, InviteRegisterInput{
		Login: "newlogin", Password: "passw0rd1", DisplayName: "New Name",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != disabledUserID {
		t.Errorf("user ID=%d want original disabled user ID=%d", u.ID, disabledUserID)
	}
	if u.Login != "newlogin" || u.DisplayName != "New Name" {
		t.Errorf("login=%q display=%q want updated", u.Login, u.DisplayName)
	}
	if u.Role != "operator" {
		t.Errorf("role=%q want invitation role", u.Role)
	}
	if u.Disabled {
		t.Error("user must be re-enabled")
	}
	if !u.EmailVerified {
		t.Error("email_verified must be set")
	}
	if jwt == "" {
		t.Error("empty session token")
	}

	// login with new password must work
	if _, _, err := p.Login("disabled@example.com", "passw0rd1"); err != nil {
		t.Errorf("login with new password: %v", err)
	}
	// login with old password must fail
	if _, _, err := p.Login("disabled@example.com", "oldpass1"); err == nil {
		t.Error("old password must not work")
	}

	// membership must be created
	m, err := p.GetMembership(u.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Role != "operator" || m.Status != "active" {
		t.Errorf("membership=%+v", m)
	}
}

// ─── CreateInvitation edge cases ─────────────────────────────────────────────

func TestCreateInvitationInvalidEmail(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	for _, email := range []string{"", "notanemail", "  "} {
		if _, _, err := p.CreateInvitation(1, inviter, email, "viewer", "link"); err == nil {
			t.Errorf("email=%q: expected error", email)
		}
	}
}

func TestCreateInvitationInvalidRole(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	if _, _, err := p.CreateInvitation(1, inviter, "x@example.com", "superadmin", "link"); err == nil || !strings.Contains(err.Error(), "invalid role") {
		t.Fatalf("err=%v want invalid-role", err)
	}
}

func TestCreateInvitationNonAdmin(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	newAccountAdmin(t, p, "owner@example.com", "owner")
	// create a non-admin user in account 1
	nonAdmin := newPlainUser(t, p, "peon@example.com", "peon")
	if _, err := p.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at) VALUES(?,1,'viewer','active',?)`, nonAdmin, db.NowMS()); err != nil {
		t.Fatal(err)
	}

	if _, _, err := p.CreateInvitation(1, nonAdmin, "target@example.com", "viewer", "link"); err == nil || !strings.Contains(err.Error(), "only account admins") {
		t.Fatalf("err=%v want admin-only", err)
	}
}

func TestCreateInvitationAlreadyMember(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	// invite an email that's already a member
	if _, _, err := p.CreateInvitation(1, inviter, "owner@example.com", "viewer", "link"); err == nil || !strings.Contains(err.Error(), "already a member") {
		t.Fatalf("err=%v want already-member", err)
	}
}

func TestCreateInvitationDuplicatePending(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	if _, _, err := p.CreateInvitation(1, inviter, "dup@example.com", "viewer", "link"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.CreateInvitation(1, inviter, "dup@example.com", "operator", "link"); err == nil || !strings.Contains(err.Error(), "pending invitation already exists") {
		t.Fatalf("err=%v want duplicate-pending", err)
	}
}

func TestCreateInvitationInvalidDeliveryMethod(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	if _, _, err := p.CreateInvitation(1, inviter, "x@example.com", "viewer", "sms"); err == nil || !strings.Contains(err.Error(), "delivery_method") {
		t.Fatalf("err=%v want delivery-method error", err)
	}
}

// ── RevokeInvitation ────────────────────────────────────────────────────────

func TestRevokeInvitation(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "revoke@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	var invID int64
	_ = p.DB.QueryRow(`SELECT id FROM account_invitations WHERE token_hash=?`, hashInviteToken(token)).Scan(&invID)

	if err := p.RevokeInvitation(1, invID); err != nil {
		t.Fatal(err)
	}

	var status string
	_ = p.DB.QueryRow(`SELECT status FROM account_invitations WHERE id=?`, invID).Scan(&status)
	if status != "revoked" {
		t.Errorf("status=%q want revoked", status)
	}

	// accept after revoke must fail
	user := newPlainUser(t, p, "revoke@example.com", "revoke")
	if _, err := p.AcceptInvitation(token, user); err == nil || !strings.Contains(err.Error(), "no longer pending") {
		t.Fatalf("err=%v want no-longer-pending after revoke", err)
	}
}

func TestRevokeInvitationAlreadyAccepted(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	user := newPlainUser(t, p, "acc2@example.com", "acc2")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "acc2@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	if _, err := p.AcceptInvitation(token, user); err != nil {
		t.Fatal(err)
	}

	var invID int64
	_ = p.DB.QueryRow(`SELECT id FROM account_invitations WHERE token_hash=?`, hashInviteToken(token)).Scan(&invID)

	if err := p.RevokeInvitation(1, invID); err == nil || !strings.Contains(err.Error(), "not pending") {
		t.Fatalf("err=%v want not-pending", err)
	}
}

func TestRevokeInvitationWrongAccount(t *testing.T) {
	p := newTestPlatform(t, config.Config{})

	now := db.NowMS()
	if _, err := p.DB.Exec(`INSERT INTO accounts(id, slug, name, status, created_at, updated_at) VALUES(2, 'other', 'Other', 'active', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}

	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "x@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	var invID int64
	_ = p.DB.QueryRow(`SELECT id FROM account_invitations WHERE token_hash=?`, hashInviteToken(token)).Scan(&invID)

	// revoke from wrong account
	if err := p.RevokeInvitation(2, invID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v want not-found (wrong account)", err)
	}
}

// ─── ResendInvitation ────────────────────────────────────────────────────────

func TestResendInvitation(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "resend@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	oldToken := inviteTokenFromURL(t, inviteURL)

	var invID int64
	_ = p.DB.QueryRow(`SELECT id FROM account_invitations WHERE token_hash=?`, hashInviteToken(oldToken)).Scan(&invID)

	newURL, err := p.ResendInvitation(1, invID)
	if err != nil {
		t.Fatal(err)
	}
	newToken := inviteTokenFromURL(t, newURL)
	if newToken == oldToken {
		t.Error("resend must generate a new token")
	}

	// old token must no longer work
	if _, err := p.GetInvitation(oldToken); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("old token should be invalid after resend: err=%v", err)
	}

	// new token must work
	info, err := p.GetInvitation(newToken)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "pending" {
		t.Errorf("status=%q want pending", info.Status)
	}
}

func TestResendInvitationNotPending(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	user := newPlainUser(t, p, "done2@example.com", "done2")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "done2@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	if _, err := p.AcceptInvitation(token, user); err != nil {
		t.Fatal(err)
	}

	var invID int64
	_ = p.DB.QueryRow(`SELECT id FROM account_invitations WHERE token_hash=?`, hashInviteToken(token)).Scan(&invID)

	if _, err := p.ResendInvitation(1, invID); err == nil || !strings.Contains(err.Error(), "no longer pending") {
		t.Fatalf("err=%v want no-longer-pending", err)
	}
}

// ─── ListInvitations ─────────────────────────────────────────────────────────

func TestListInvitations(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")

	_, url1, _ := p.CreateInvitation(1, inviter, "a@example.com", "viewer", "link")
	_, _, _ = p.CreateInvitation(1, inviter, "b@example.com", "operator", "link")
	token1 := inviteTokenFromURL(t, url1)

	// accept one via register
	if _, _, err := p.RegisterInvitedUser(token1, InviteRegisterInput{Login: "aaa", Password: "passw0rd1"}); err != nil {
		t.Fatal(err)
	}

	list, err := p.ListInvitations(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("count=%d want 2", len(list))
	}

	// one accepted, one pending
	statuses := map[string]string{}
	for _, inv := range list {
		statuses[inv.Email] = inv.Status
	}
	if statuses["a@example.com"] != "accepted" {
		t.Errorf("a@example.com status=%q want accepted", statuses["a@example.com"])
	}
	if statuses["b@example.com"] != "pending" {
		t.Errorf("b@example.com status=%q want pending", statuses["b@example.com"])
	}
}

func TestListInvitationsMarksExpired(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	inv, _, err := p.CreateInvitation(1, inviter, "old@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	// backdate
	if _, err := p.DB.Exec(`UPDATE account_invitations SET expires_at=? WHERE id=?`, db.NowMS()-1, inv.ID); err != nil {
		t.Fatal(err)
	}

	list, err := p.ListInvitations(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("count=%d want 1", len(list))
	}
	if list[0].Status != "expired" {
		t.Errorf("status=%q want expired (marked on read)", list[0].Status)
	}
}

func TestListInvitationsEmpty(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	list, err := p.ListInvitations(1)
	if err != nil {
		t.Fatal(err)
	}
	if list == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(list) != 0 {
		t.Errorf("count=%d want 0", len(list))
	}
}

// ─── Cross-account / global email uniqueness ─────────────────────────────────

func TestAcceptInvitationUserInDifferentAccount(t *testing.T) {
	// User has email X in account 1, gets invited to account 2 with same email.
	// Accept should succeed — user joins account 2 as well.
	p := newTestPlatform(t, config.Config{})

	now := db.NowMS()
	if _, err := p.DB.Exec(`INSERT INTO accounts(id, slug, name, status, created_at, updated_at) VALUES(2, 'other', 'Other Account', 'active', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}

	newAccountAdmin(t, p, "owner@example.com", "owner")
	admin2 := newAccountAdmin(t, p, "owner2@example.com", "owner2")
	// fix admin2 to be in account 2
	if _, err := p.DB.Exec(`UPDATE account_memberships SET account_id=2 WHERE user_id=?`, admin2); err != nil {
		t.Fatal(err)
	}

	// user exists in account 1
	user := newPlainUser(t, p, "multi@example.com", "multi")
	if _, err := p.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at) VALUES(?,1,'viewer','active',?)`, user, now); err != nil {
		t.Fatal(err)
	}

	// invite same email to account 2
	_, inviteURL, err := p.CreateInvitation(2, admin2, "multi@example.com", "operator", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	m, err := p.AcceptInvitation(token, user)
	if err != nil {
		t.Fatal(err)
	}
	if m.AccountID != 2 || m.Role != "operator" {
		t.Errorf("membership=%+v want account=2 role=operator", m)
	}

	// user should now be in both accounts
	m1, err := p.GetMembership(user, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m1.AccountID != 1 {
		t.Errorf("user lost account 1 membership")
	}
	m2, err := p.GetMembership(user, 2)
	if err != nil {
		t.Fatal(err)
	}
	if m2.AccountID != 2 || m2.Role != "operator" {
		t.Errorf("account 2 membership=%+v", m2)
	}
}

func TestRegisterInvitedUserSingleUse(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	_, inviteURL, err := p.CreateInvitation(1, inviter, "once@example.com", "viewer", "link")
	if err != nil {
		t.Fatal(err)
	}
	token := inviteTokenFromURL(t, inviteURL)

	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "once", Password: "passw0rd1"}); err != nil {
		t.Fatal(err)
	}
	// second register with same token
	if _, _, err := p.RegisterInvitedUser(token, InviteRegisterInput{Login: "twice", Password: "passw0rd1"}); err == nil {
		t.Error("second register must fail")
	}
}

// ─── CreateMember ────────────────────────────────────────────────────────────

func TestCreateMemberHappyPath(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")

	u, err := p.CreateMember(1, inviter, CreateMemberInput{
		Login: "direct", Email: "direct@example.com", Password: "passw0rd1",
		Role: "viewer", DisplayName: "Direct User",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "direct@example.com" || u.Login != "direct" {
		t.Errorf("user=%+v", u)
	}

	m, err := p.GetMembership(u.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Role != "viewer" || m.Status != "active" {
		t.Errorf("membership=%+v", m)
	}
}

func TestCreateMemberNonAdmin(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	newAccountAdmin(t, p, "owner@example.com", "owner")
	nonAdmin := newPlainUser(t, p, "peon2@example.com", "peon2")
	if _, err := p.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at) VALUES(?,1,'viewer','active',?)`, nonAdmin, db.NowMS()); err != nil {
		t.Fatal(err)
	}

	if _, err := p.CreateMember(1, nonAdmin, CreateMemberInput{
		Login: "xuser", Email: "x@example.com", Password: "passw0rd1", Role: "viewer",
	}); err == nil || !strings.Contains(err.Error(), "only account admins") {
		t.Fatalf("err=%v want admin-only", err)
	}
}

func TestCreateMemberEmailOrLoginTaken(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	inviter := newAccountAdmin(t, p, "owner@example.com", "owner")
	newPlainUser(t, p, "exists@example.com", "exists")

	if _, err := p.CreateMember(1, inviter, CreateMemberInput{
		Login: "exists", Email: "exists@example.com", Password: "passw0rd1", Role: "viewer",
	}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v want already-exists", err)
	}
}
