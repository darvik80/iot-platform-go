package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

const inviteTTL = 72 * time.Hour

// CreateInvitation creates a new invitation for email to join accountID with role.
// Returns the invitation, the invite URL, and any error.
func (p *Platform) CreateInvitation(accountID, invitedBy int64, email, role, deliveryMethod string) (*models.AccountInvitation, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, "", errors.New("valid email required")
	}
	switch models.Role(role) {
	case models.RoleAdmin, models.RoleOperator, models.RoleViewer:
	default:
		return nil, "", errors.New("invalid role")
	}
	if deliveryMethod == "" {
		deliveryMethod = "email"
	}
	if deliveryMethod != "email" && deliveryMethod != "link" {
		return nil, "", errors.New("delivery_method must be email or link")
	}

	// Verify inviter is admin in account
	m, err := p.GetMembership(invitedBy, accountID)
	if err != nil || m.Role != models.RoleAdmin {
		return nil, "", errors.New("only account admins can invite")
	}

	// Check if user is already a member
	var existingMember int
	_ = p.DB.QueryRow(`SELECT COUNT(1) FROM account_memberships WHERE account_id=? AND user_id IN
		(SELECT id FROM users WHERE lower(email)=?)`, accountID, email).Scan(&existingMember)
	if existingMember > 0 {
		return nil, "", errors.New("user is already a member of this account")
	}

	// Check for pending invitation
	var pendingCount int
	_ = p.DB.QueryRow(`SELECT COUNT(1) FROM account_invitations WHERE account_id=? AND lower(email)=? AND status='pending' AND expires_at>?`,
		accountID, email, db.NowMS()).Scan(&pendingCount)
	if pendingCount > 0 {
		return nil, "", errors.New("pending invitation already exists for this email")
	}

	// Generate token
	rawToken, err := auth.GenerateAccessToken()
	if err != nil {
		return nil, "", err
	}
	tokenHash := hashInviteToken(rawToken)
	now := db.NowMS()
	expiresAt := now + inviteTTL.Milliseconds()

	res, err := p.DB.Exec(`INSERT INTO account_invitations(account_id, email, role, token_hash, status, delivery_method, invited_by, expires_at, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		accountID, email, role, tokenHash, "pending", deliveryMethod, invitedBy, expiresAt, now, now)
	if err != nil {
		return nil, "", err
	}
	id, _ := res.LastInsertId()

	inv := &models.AccountInvitation{
		ID:             id,
		AccountID:      accountID,
		Email:          email,
		Role:           models.Role(role),
		TokenHash:      tokenHash,
		Status:         "pending",
		DeliveryMethod: deliveryMethod,
		InvitedBy:      invitedBy,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	inviteURL := p.inviteURL(rawToken)

	// Send email if delivery method is email
	if deliveryMethod == "email" {
		var accName string
		_ = p.DB.QueryRow(`SELECT name FROM accounts WHERE id=?`, accountID).Scan(&accName)
		if err := p.Mailer().SendInvitation(email, accName, role, inviteURL); err != nil {
			p.Log.Warn("invitation email failed", "err", err, "email", email)
		}
	}

	return inv, inviteURL, nil
}

// GetInvitation returns public info about an invitation by token.
func (p *Platform) GetInvitation(token string) (*models.InvitationInfo, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("missing token")
	}
	h := hashInviteToken(token)
	var info models.InvitationInfo
	var status string
	err := p.DB.QueryRow(`SELECT a.name, a.slug, i.email, i.role, i.status, i.expires_at
		FROM account_invitations i JOIN accounts a ON a.id = i.account_id
		WHERE i.token_hash=?`, h).Scan(
		&info.AccountName, &info.AccountSlug, &info.Email, &info.Role, &status, &info.ExpiresAt)
	if err != nil {
		return nil, errors.New("invitation not found")
	}
	// Check expiration
	if info.ExpiresAt < db.NowMS() && status == "pending" {
		status = "expired"
	}
	info.Status = status
	return &info, nil
}

// AcceptInvitation validates the token, creates membership, and marks invitation accepted.
func (p *Platform) AcceptInvitation(token string, userID int64) (*models.AccountMembership, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("missing token")
	}
	h := hashInviteToken(token)
	now := db.NowMS()

	var inv models.AccountInvitation
	err := p.DB.QueryRow(`SELECT id, account_id, email, role, status, expires_at, invited_by
		FROM account_invitations WHERE token_hash=?`, h).Scan(
		&inv.ID, &inv.AccountID, &inv.Email, &inv.Role, &inv.Status, &inv.ExpiresAt, &inv.InvitedBy)
	if err != nil {
		return nil, errors.New("invitation not found")
	}
	if inv.Status != "pending" {
		return nil, errors.New("invitation no longer pending")
	}
	if inv.ExpiresAt < now {
		_, _ = p.DB.Exec(`UPDATE account_invitations SET status='expired', updated_at=? WHERE id=?`, now, inv.ID)
		return nil, errors.New("invitation expired")
	}

	// Verify user email matches invitation
	u, err := p.GetUser(userID)
	if err != nil {
		return nil, err
	}
	if strings.ToLower(strings.TrimSpace(u.Email)) != inv.Email {
		return nil, errors.New("email does not match invitation")
	}

	// Check if already a member
	var existingCount int
	_ = p.DB.QueryRow(`SELECT COUNT(1) FROM account_memberships WHERE account_id=? AND user_id=?`,
		inv.AccountID, userID).Scan(&existingCount)
	if existingCount > 0 {
		return nil, errors.New("already a member")
	}

	// Create membership
	_, err = p.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at, invited_by)
		VALUES(?,?,?,?,?,?)`,
		userID, inv.AccountID, inv.Role, "active", now, inv.InvitedBy)
	if err != nil {
		return nil, err
	}

	// Mark invitation accepted
	_, _ = p.DB.Exec(`UPDATE account_invitations SET status='accepted', updated_at=? WHERE id=?`, now, inv.ID)

	return &models.AccountMembership{
		UserID:    userID,
		AccountID: inv.AccountID,
		Role:      inv.Role,
		Status:    "active",
		JoinedAt:  now,
		InvitedBy: &inv.InvitedBy,
	}, nil
}

// InviteRegisterInput is the account-setup form for a person holding an
// invitation link. Email is not accepted here — it always comes from the
// invitation, so a typo can never strand the acceptance.
type InviteRegisterInput struct {
	Login       string `json:"login"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// RegisterInvitedUser creates the invited user (email fixed by the invitation,
// password chosen by them), adds the membership, marks the invitation
// accepted and returns a session token. Works even when public registration is
// disabled: an invitation is an explicit admin grant.
func (p *Platform) RegisterInvitedUser(token string, in InviteRegisterInput) (*models.User, string, error) {
	in.Login = strings.ToLower(strings.TrimSpace(in.Login))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if !loginRe.MatchString(in.Login) {
		return nil, "", errors.New("invalid login: 3-32 chars, start with letter, [a-zA-Z0-9._-]")
	}
	if len(in.Password) < 8 {
		return nil, "", errors.New("password must be at least 8 characters")
	}
	hasLetter, hasDigit := false, false
	for _, r := range in.Password {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return nil, "", errors.New("password must contain at least one letter and one digit")
	}

	h := hashInviteToken(strings.TrimSpace(token))
	// Resolve everything outside the tx: SQLite runs on a single pooled
	// connection, so p.DB queries inside a tx would deadlock.
	var inv models.AccountInvitation
	err := p.DB.QueryRow(`SELECT id, account_id, email, role, status, expires_at, invited_by
		FROM account_invitations WHERE token_hash=?`, h).Scan(
		&inv.ID, &inv.AccountID, &inv.Email, &inv.Role, &inv.Status, &inv.ExpiresAt, &inv.InvitedBy)
	if err != nil {
		return nil, "", errors.New("invitation not found")
	}
	if inv.Status != "pending" {
		return nil, "", errors.New("invitation no longer pending")
	}
	now := db.NowMS()
	if inv.ExpiresAt < now {
		_, _ = p.DB.Exec(`UPDATE account_invitations SET status='expired', updated_at=? WHERE id=?`, now, inv.ID)
		return nil, "", errors.New("invitation expired")
	}

	// Check for existing user with this email.
	// - Active user  → block (must sign in to accept)
	// - Disabled user → reactivate via invitation (account recovery)
	// - No user       → create new
	var existingID int64
	var existingDisabled int
	err = p.DB.QueryRow(`SELECT id, COALESCE(disabled,0) FROM users WHERE lower(email)=?`, inv.Email).
		Scan(&existingID, &existingDisabled)
	if err == nil && existingDisabled == 0 {
		return nil, "", errors.New("an account with this email already exists — sign in to accept the invitation")
	}

	if in.DisplayName == "" {
		in.DisplayName = in.Login
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, "", err
	}

	tx, err := p.DB.Begin()
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	var userID int64
	if err == nil && existingDisabled == 1 {
		// Reactivate disabled user: reset password, login, role, enable.
		_, err = tx.Exec(`UPDATE users SET password_hash=?, login=?, display_name=?, role=?, disabled=0, email_verified=1, updated_at=? WHERE id=?`,
			hash, in.Login, in.DisplayName, inv.Role, now, existingID)
		if err != nil {
			return nil, "", err
		}
		userID = existingID
	} else {
		res, err := tx.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?)`,
			inv.Email, hash, inv.Role, in.Login, in.DisplayName, 1, now, now)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return nil, "", errors.New("login already taken")
			}
			return nil, "", err
		}
		userID, _ = res.LastInsertId()
	}

	_, err = tx.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at, invited_by)
		VALUES(?,?,?,?,?,?)`,
		userID, inv.AccountID, inv.Role, "active", now, inv.InvitedBy)
	if err != nil {
		return nil, "", err
	}

	// Guard the pending→accepted transition inside the tx: a concurrent
	// accept or revoke makes this UPDATE a no-op and we roll back.
	ures, err := tx.Exec(`UPDATE account_invitations SET status='accepted', updated_at=? WHERE id=? AND status='pending'`,
		now, inv.ID)
	if err != nil {
		return nil, "", err
	}
	if n, _ := ures.RowsAffected(); n == 0 {
		return nil, "", errors.New("invitation no longer pending")
	}

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	u, err := p.GetUser(userID)
	if err != nil {
		return nil, "", err
	}
	refs, _, err := p.LoadAccountRefs(userID)
	if err != nil {
		return nil, "", err
	}
	jwt, err := auth.IssueJWT(p.Cfg.Auth.JWTSecret, *u, refs, inv.AccountID, 24*time.Hour)
	if err != nil {
		return nil, "", err
	}
	return u, jwt, nil
}

// ResendInvitation generates a new token for an existing invitation and revokes the old one.
func (p *Platform) ResendInvitation(accountID, invitationID int64) (string, error) {
	now := db.NowMS()
	var email, role, status string
	var expiresAt int64
	err := p.DB.QueryRow(`SELECT email, role, status, expires_at FROM account_invitations WHERE id=? AND account_id=?`,
		invitationID, accountID).Scan(&email, &role, &status, &expiresAt)
	if err != nil {
		return "", errors.New("invitation not found")
	}
	if status != "pending" {
		return "", errors.New("invitation no longer pending")
	}

	// Generate new token
	rawToken, err := auth.GenerateAccessToken()
	if err != nil {
		return "", err
	}
	newHash := hashInviteToken(rawToken)
	newExpires := now + inviteTTL.Milliseconds()

	_, err = p.DB.Exec(`UPDATE account_invitations SET token_hash=?, expires_at=?, updated_at=? WHERE id=?`,
		newHash, newExpires, now, invitationID)
	if err != nil {
		return "", err
	}

	inviteURL := p.inviteURL(rawToken)

	// Send email
	var accName string
	_ = p.DB.QueryRow(`SELECT name FROM accounts WHERE id=?`, accountID).Scan(&accName)
	if err := p.Mailer().SendInvitation(email, accName, role, inviteURL); err != nil {
		p.Log.Warn("invitation email failed", "err", err, "email", email)
	}

	return inviteURL, nil
}

// RevokeInvitation marks an invitation as revoked.
func (p *Platform) RevokeInvitation(accountID, invitationID int64) error {
	now := db.NowMS()
	res, err := p.DB.Exec(`UPDATE account_invitations SET status='revoked', updated_at=? WHERE id=? AND account_id=? AND status='pending'`,
		now, invitationID, accountID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("invitation not found or not pending")
	}
	return nil
}

// ListInvitations returns all invitations for an account.
func (p *Platform) ListInvitations(accountID int64) ([]models.AccountInvitation, error) {
	rows, err := p.DB.Query(`SELECT id, account_id, email, role, status, delivery_method, invited_by, expires_at, created_at, updated_at
		FROM account_invitations WHERE account_id=? ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AccountInvitation
	for rows.Next() {
		var inv models.AccountInvitation
		if err := rows.Scan(&inv.ID, &inv.AccountID, &inv.Email, &inv.Role, &inv.Status,
			&inv.DeliveryMethod, &inv.InvitedBy, &inv.ExpiresAt, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
			return nil, err
		}
		// Mark expired invitations
		if inv.Status == "pending" && inv.ExpiresAt < db.NowMS() {
			inv.Status = "expired"
		}
		out = append(out, inv)
	}
	if out == nil {
		out = []models.AccountInvitation{}
	}
	return out, rows.Err()
}

// CreateMember creates a new user and adds them as a member of the account directly.
// Skips email verification. Validates password policy.
func (p *Platform) CreateMember(accountID, invitedBy int64, in CreateMemberInput) (*models.User, error) {
	in.Login = strings.ToLower(strings.TrimSpace(in.Login))
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)

	if !loginRe.MatchString(in.Login) {
		return nil, errors.New("invalid login: 3-32 chars, start with letter, [a-zA-Z0-9._-]")
	}
	if !strings.Contains(in.Email, "@") || len(in.Email) < 5 {
		return nil, errors.New("invalid email")
	}
	if len(in.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}
	// Password must contain letter + digit
	hasLetter, hasDigit := false, false
	for _, r := range in.Password {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return nil, errors.New("password must contain at least one letter and one digit")
	}

	switch models.Role(in.Role) {
	case models.RoleAdmin, models.RoleOperator, models.RoleViewer:
	default:
		return nil, errors.New("invalid role")
	}

	// Verify creator is admin
	m, err := p.GetMembership(invitedBy, accountID)
	if err != nil || m.Role != models.RoleAdmin {
		return nil, errors.New("only account admins can create members")
	}

	if in.DisplayName == "" {
		in.DisplayName = in.Login
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	now := db.NowMS()

	tx, err := p.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		in.Email, hash, models.Role(in.Role), in.Login, in.DisplayName, 1, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("login or email already exists")
		}
		return nil, err
	}
	userID, _ := res.LastInsertId()

	_, err = tx.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at, invited_by)
		VALUES(?,?,?,?,?,?)`,
		userID, accountID, in.Role, "active", now, invitedBy)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return p.GetUser(userID)
}

// CreateMemberInput is the input for manual member creation.
type CreateMemberInput struct {
	Login       string `json:"login"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func hashInviteToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (p *Platform) inviteURL(rawToken string) string {
	return fmt.Sprintf("%s/invite/%s", p.Cfg.Platform.PublicBaseURL, url.PathEscape(rawToken))
}
