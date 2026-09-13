package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/mail"
	"cogitor-iot-platform/internal/models"
)

var loginRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`)

func (p *Platform) Mailer() *mail.Sender {
	return &mail.Sender{Cfg: p.Cfg, Log: p.Log}
}

// EnsureAdmin checks if an admin user exists and updates it if needed.
// For fresh databases, use the setup wizard instead.
// This is only called when the database already has data (migration case).
func (p *Platform) EnsureAdmin() error {
	var n int
	if err := p.DB.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		// No users exist — this should have been handled by setup wizard
		// But if we're here, it means accounts exist but no users (edge case)
		// Don't create anything, let the system work with existing data
		return nil
	}

	// Ensure migration columns filled for bootstrap admin if needed.
	_, _ = p.DB.Exec(`UPDATE users SET login=? WHERE (login IS NULL OR login='') AND email=?`,
		p.Cfg.AdminLogin, p.Cfg.AdminEmail)
	_, _ = p.DB.Exec(`UPDATE users SET email_verified=1 WHERE email=?`, p.Cfg.AdminEmail)

	return nil
}

type RegisterInput struct {
	Login       string `json:"login"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	AccountSlug string `json:"account_slug"` // optional, auto-generated from login if empty
}

func (p *Platform) Register(in RegisterInput) (*models.User, error) {
	if !p.Cfg.RegisterEnabled {
		return nil, errors.New("registration disabled")
	}
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
	if in.DisplayName == "" {
		in.DisplayName = in.Login
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	now := db.NowMS()
	verified := 0
	if !p.Cfg.RequireEmailVerify {
		verified = 1
	}

	tx, err := p.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Create user
	res, err := tx.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		in.Email, hash, p.Cfg.RegisterDefaultRole, in.Login, in.DisplayName, verified, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("login or email already registered")
		}
		return nil, err
	}
	userID, _ := res.LastInsertId()

	// 2. Determine account slug
	var slug string
	if in.AccountSlug != "" {
		slug = normalizeSlug(in.AccountSlug)
		if slug == "" {
			return nil, errors.New("invalid account slug: only lowercase letters, digits, and hyphens allowed")
		}
		if len(slug) > 63 {
			slug = slug[:63]
		}
		// Check uniqueness
		var count int
		_ = p.DB.QueryRow(`SELECT COUNT(1) FROM accounts WHERE slug=?`, slug).Scan(&count)
		if count > 0 {
			return nil, errors.New("account slug already taken")
		}
	} else {
		slug = p.generateAccountSlug(in.Login)
	}

	// 3. Create personal account
	accRes, err := tx.Exec(`INSERT INTO accounts(slug, name, settings_json, status, created_at, updated_at)
		VALUES(?,?,?,'active',?,?)`,
		slug, in.DisplayName, "{}", now, now)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	accountID, _ := accRes.LastInsertId()

	// 4. Create membership (user is admin of their own account)
	_, err = tx.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at)
		VALUES(?,?,?,'active',?)`,
		userID, accountID, models.RoleAdmin, now)
	if err != nil {
		return nil, fmt.Errorf("create membership: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	u, err := p.GetUser(userID)
	if err != nil {
		return nil, err
	}
	if p.Cfg.RequireEmailVerify {
		if err := p.sendVerificationEmail(u); err != nil {
			p.Log.Warn("verification email failed", "err", err)
			return u, fmt.Errorf("registered but email send failed: %w", err)
		}
	}
	return u, nil
}

// generateAccountSlug creates a unique account slug from a login name.
// Converts to lowercase, replaces non-alphanumeric characters with hyphens,
// and appends a numeric suffix if the slug is already taken.
func (p *Platform) generateAccountSlug(login string) string {
	base := strings.ToLower(login)
	// Replace non-alphanumeric characters with hyphens
	var b strings.Builder
	for _, ch := range base {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
		} else {
			b.WriteByte('-')
		}
	}
	base = strings.Trim(b.String(), "-")
	// Collapse consecutive hyphens
	for strings.Contains(base, "--") {
		base = strings.ReplaceAll(base, "--", "-")
	}
	if base == "" {
		base = "user"
	}
	if len(base) > 56 {
		base = base[:56]
	}

	// Check uniqueness, append suffix if taken
	candidate := base
	for i := 1; i <= 100; i++ {
		var count int
		_ = p.DB.QueryRow(`SELECT COUNT(1) FROM accounts WHERE slug=?`, candidate).Scan(&count)
		if count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	// Fallback: add timestamp
	return fmt.Sprintf("%s-%d", base, db.NowMS())
}

// normalizeSlug converts a user-provided slug to a valid format.
// Returns empty string if the slug contains invalid characters.
func normalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
		} else if ch == '-' {
			b.WriteRune(ch)
		} else {
			return "" // invalid character
		}
	}
	result := strings.Trim(b.String(), "-")
	// Collapse consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return result
}

// CheckSlugAvailable returns true if the slug is not taken.
func (p *Platform) CheckSlugAvailable(slug string) bool {
	slug = normalizeSlug(slug)
	if slug == "" {
		return false
	}
	var count int
	_ = p.DB.QueryRow(`SELECT COUNT(1) FROM accounts WHERE slug=?`, slug).Scan(&count)
	return count == 0
}

func (p *Platform) sendVerificationEmail(u *models.User) error {
	raw, err := auth.GenerateAccessToken()
	if err != nil {
		return err
	}
	now := db.NowMS()
	expires := now + 24*3600*1000
	_, err = p.DB.Exec(`INSERT INTO email_verifications(user_id, token_hash, expires_at, created_at) VALUES(?,?,?,?)`,
		u.ID, auth.HashToken(raw), expires, now)
	if err != nil {
		return err
	}
	link := fmt.Sprintf("%s/api/v1/auth/verify-email?token=%s", p.Cfg.PublicBaseURL, url.QueryEscape(raw))
	return p.Mailer().SendVerification(u.Email, u.Login, link)
}

func (p *Platform) ResendVerification(emailOrLogin string) error {
	u, err := p.FindUserByLoginOrEmail(emailOrLogin)
	if err != nil {
		// don't leak existence
		return nil
	}
	if u.EmailVerified {
		return nil
	}
	return p.sendVerificationEmail(u)
}

func (p *Platform) VerifyEmail(token string) (*models.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("missing token")
	}
	h := auth.HashToken(token)
	now := db.NowMS()
	var userID int64
	err := p.DB.QueryRow(`SELECT user_id FROM email_verifications WHERE token_hash=? AND expires_at>=?`, h, now).Scan(&userID)
	if err != nil {
		return nil, errors.New("invalid or expired verification token")
	}
	_, err = p.DB.Exec(`UPDATE users SET email_verified=1, updated_at=? WHERE id=?`, now, userID)
	if err != nil {
		return nil, err
	}
	_, _ = p.DB.Exec(`DELETE FROM email_verifications WHERE user_id=?`, userID)
	return p.GetUser(userID)
}

func (p *Platform) Login(identity, password string) (*models.User, string, error) {
	identity = strings.TrimSpace(identity)
	u, err := p.FindUserByLoginOrEmail(identity)
	if err != nil {
		return nil, "", errors.New("invalid credentials")
	}
	if u.Disabled {
		return nil, "", errors.New("account disabled")
	}
	if u.PasswordHash == "" {
		return nil, "", errors.New("use Google sign-in for this account")
	}
	if !auth.CheckPassword(u.PasswordHash, password) {
		return nil, "", errors.New("invalid credentials")
	}
	if p.Cfg.RequireEmailVerify && !u.EmailVerified {
		return nil, "", errors.New("email not verified")
	}
	refs, activeID, err := p.LoadAccountRefs(u.ID)
	if err != nil {
		return nil, "", err
	}
	if len(refs) == 0 {
		return nil, "", errors.New("no account membership — contact administrator")
	}
	tok, err := auth.IssueJWT(p.Cfg.JWTSecret, *u, refs, activeID, 24*time.Hour)
	if err != nil {
		return nil, "", err
	}
	return u, tok, nil
}

// LoadAccountRefs queries account memberships for a user and returns
// AccountRef slice plus the default active account ID (first membership).
func (p *Platform) LoadAccountRefs(userID int64) ([]auth.AccountRef, int64, error) {
	rows, err := p.DB.Query(`SELECT m.account_id, a.slug, m.role
		FROM account_memberships m JOIN accounts a ON a.id = m.account_id
		WHERE m.user_id = ? AND m.status = 'active' AND a.status = 'active'
		ORDER BY m.joined_at`, userID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var refs []auth.AccountRef
	var firstID int64
	for rows.Next() {
		var r auth.AccountRef
		if err := rows.Scan(&r.AccountID, &r.AccountSlug, &r.Role); err != nil {
			return nil, 0, err
		}
		if firstID == 0 {
			firstID = r.AccountID
		}
		refs = append(refs, r)
	}
	if refs == nil {
		refs = []auth.AccountRef{}
	}
	return refs, firstID, rows.Err()
}

func (p *Platform) FindUserByLoginOrEmail(identity string) (*models.User, error) {
	identity = strings.ToLower(strings.TrimSpace(identity))
	row := p.DB.QueryRow(userSelect+` FROM users WHERE lower(email)=? OR lower(login)=?`, identity, identity)
	return scanUser(row)
}

func (p *Platform) GetUser(id int64) (*models.User, error) {
	row := p.DB.QueryRow(userSelect+` FROM users WHERE id=?`, id)
	return scanUser(row)
}

func (p *Platform) ListUsers() ([]models.User, error) {
	rows, err := p.DB.Query(userSelect + ` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	if out == nil {
		out = []models.User{}
	}
	return out, nil
}

type UpdateUserInput struct {
	Role        string `json:"role"`
	Disabled    *bool  `json:"disabled"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"` // if set, change password
}

func (p *Platform) UpdateUser(id int64, in UpdateUserInput) (*models.User, error) {
	u, err := p.GetUser(id)
	if err != nil {
		return nil, err
	}
	if in.Role != "" {
		role := models.Role(in.Role)
		switch role {
		case models.RoleAdmin, models.RoleOperator, models.RoleViewer:
			u.Role = role
		default:
			return nil, errors.New("invalid role")
		}
	}
	if in.Disabled != nil {
		u.Disabled = *in.Disabled
	}
	if in.DisplayName != "" {
		u.DisplayName = in.DisplayName
	}
	if in.Email != "" {
		u.Email = in.Email
	}
	// Hash new password if provided
	newHash := ""
	if in.Password != "" {
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			return nil, errors.New("failed to hash password")
		}
		newHash = hash
	}
	dis := 0
	if u.Disabled {
		dis = 1
	}
	if newHash != "" {
		_, err = p.DB.Exec(`UPDATE users SET role=?, disabled=?, display_name=?, email=?, password_hash=?, updated_at=? WHERE id=?`,
			u.Role, dis, u.DisplayName, u.Email, newHash, db.NowMS(), id)
	} else {
		_, err = p.DB.Exec(`UPDATE users SET role=?, disabled=?, display_name=?, email=?, updated_at=? WHERE id=?`,
			u.Role, dis, u.DisplayName, u.Email, db.NowMS(), id)
	}
	if err != nil {
		return nil, err
	}
	return p.GetUser(id)
}

func (p *Platform) DeleteUser(id int64) error {
	res, err := p.DB.Exec(`DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("not found")
	}
	return nil
}

// UserByAPIToken returns the user and the token's account_id (0 if legacy token without account).
func (p *Platform) UserByAPIToken(token string) (*models.User, int64, error) {
	h := auth.HashToken(token)
	var id int64
	var accID int64
	err := p.DB.QueryRow(`SELECT user_id, COALESCE(account_id,0) FROM api_tokens WHERE token_hash=?`, h).Scan(&id, &accID)
	if err != nil {
		return nil, 0, err
	}
	_, _ = p.DB.Exec(`UPDATE api_tokens SET last_used_at=? WHERE token_hash=?`, db.NowMS(), h)
	u, err := p.GetUser(id)
	return u, accID, err
}

func (p *Platform) ListAPITokens(userID int64, accountID int64) ([]models.APIToken, error) {
	var rows *sql.Rows
	var err error
	if accountID > 0 {
		rows, err = p.DB.Query(`SELECT id, user_id, COALESCE(account_id,0), name, token_prefix, created_at, last_used_at
			FROM api_tokens WHERE user_id=? AND account_id=? ORDER BY id DESC`, userID, accountID)
	} else {
		rows, err = p.DB.Query(`SELECT id, user_id, COALESCE(account_id,0), name, token_prefix, created_at, last_used_at
			FROM api_tokens WHERE user_id=? ORDER BY id DESC`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.APIToken
	for rows.Next() {
		var t models.APIToken
		var last sql.NullInt64
		var accID int64
		if err := rows.Scan(&t.ID, &t.UserID, &accID, &t.Name, &t.Prefix, &t.CreatedAt, &last); err != nil {
			return nil, err
		}
		if accID > 0 {
			t.AccountID = &accID
		}
		if last.Valid {
			v := last.Int64
			t.LastUsedAt = &v
		}
		out = append(out, t)
	}
	if out == nil {
		out = []models.APIToken{}
	}
	return out, rows.Err()
}

func (p *Platform) CreateAPIToken(userID int64, accountID int64, name string) (*models.APIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name required")
	}
	if len(name) > 64 {
		return nil, errors.New("name too long")
	}
	raw, err := auth.GenerateAccessToken()
	if err != nil {
		return nil, err
	}
	// Prefix so humans can tell API tokens from JWTs in configs.
	raw = "cip_" + raw
	now := db.NowMS()
	prefix := auth.TokenPrefix(raw)
	res, err := p.DB.Exec(`INSERT INTO api_tokens(user_id, account_id, name, token_hash, token_prefix, created_at) VALUES(?,?,?,?,?,?)`,
		userID, accountID, name, auth.HashToken(raw), prefix, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.APIToken{
		ID:        id,
		UserID:    userID,
		AccountID: &accountID,
		Name:      name,
		Token:     raw,
		Prefix:    prefix,
		CreatedAt: now,
	}, nil
}

func (p *Platform) RevokeAPIToken(userID, tokenID int64) error {
	res, err := p.DB.Exec(`DELETE FROM api_tokens WHERE id=? AND user_id=?`, tokenID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("not found")
	}
	return nil
}

const userSelect = `SELECT id, COALESCE(login,''), email, COALESCE(display_name,''), password_hash, role,
	COALESCE(email_verified,0), COALESCE(google_sub,''), COALESCE(disabled,0),
	COALESCE(avatar_url,''), created_at, updated_at`

func scanUser(row interface{ Scan(dest ...any) error }) (*models.User, error) {
	var u models.User
	var verified, disabled int
	err := row.Scan(&u.ID, &u.Login, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Role,
		&verified, &u.GoogleSub, &disabled, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("not found")
	}
	if err != nil {
		return nil, err
	}
	u.EmailVerified = verified == 1
	u.Disabled = disabled == 1
	switch {
	case u.GoogleSub != "" && u.PasswordHash != "":
		u.AuthProvider = "both"
	case u.GoogleSub != "":
		u.AuthProvider = "google"
	default:
		u.AuthProvider = "password"
	}
	return &u, nil
}

// --- Google OAuth ---

func (p *Platform) GoogleAuthURL() (string, error) {
	if !p.Cfg.GoogleEnabled() {
		return "", errors.New("google oauth not configured")
	}
	state, err := auth.GenerateAccessToken()
	if err != nil {
		return "", err
	}
	now := db.NowMS()
	_, err = p.DB.Exec(`INSERT INTO oauth_states(state, expires_at, created_at) VALUES(?,?,?)`,
		state, now+10*60*1000, now)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("client_id", p.Cfg.GoogleClientID)
	q.Set("redirect_uri", p.Cfg.GoogleCallbackURL())
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("access_type", "online")
	q.Set("include_granted_scopes", "true")
	q.Set("state", state)
	q.Set("prompt", "select_account")
	return "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode(), nil
}

func (p *Platform) GoogleCallback(ctx context.Context, code, state string) (*models.User, string, error) {
	if !p.Cfg.GoogleEnabled() {
		return nil, "", errors.New("google oauth not configured")
	}
	now := db.NowMS()
	var dummy string
	err := p.DB.QueryRow(`SELECT state FROM oauth_states WHERE state=? AND expires_at>=?`, state, now).Scan(&dummy)
	if err != nil {
		return nil, "", errors.New("invalid oauth state")
	}
	_, _ = p.DB.Exec(`DELETE FROM oauth_states WHERE state=?`, state)

	tokenURL := "https://oauth2.googleapis.com/token"
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", p.Cfg.GoogleClientID)
	form.Set("client_secret", p.Cfg.GoogleClientSecret)
	form.Set("redirect_uri", p.Cfg.GoogleCallbackURL())
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("google token exchange failed: %s", string(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, "", err
	}
	if tok.AccessToken == "" {
		return nil, "", errors.New("no access_token from google")
	}

	ureq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return nil, "", err
	}
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	uresp, err := http.DefaultClient.Do(ureq)
	if err != nil {
		return nil, "", err
	}
	defer uresp.Body.Close()
	ubody, _ := io.ReadAll(uresp.Body)
	if uresp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("google userinfo failed: %s", string(ubody))
	}
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.Unmarshal(ubody, &info); err != nil {
		return nil, "", err
	}
	if info.Sub == "" || info.Email == "" {
		return nil, "", errors.New("google account missing email")
	}

	u, err := p.upsertGoogleUser(info.Sub, info.Email, info.Name, info.EmailVerified)
	if err != nil {
		return nil, "", err
	}
	if u.Disabled {
		return nil, "", errors.New("account disabled")
	}
	refs, activeID, err := p.LoadAccountRefs(u.ID)
	if err != nil {
		return nil, "", err
	}
	if len(refs) == 0 {
		return nil, "", errors.New("no account membership — contact administrator")
	}
	jwt, err := auth.IssueJWT(p.Cfg.JWTSecret, *u, refs, activeID, 24*time.Hour)
	if err != nil {
		return nil, "", err
	}
	return u, jwt, nil
}

func (p *Platform) upsertGoogleUser(sub, email, name string, emailVerified bool) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	now := db.NowMS()

	// Existing by google_sub
	row := p.DB.QueryRow(`SELECT id FROM users WHERE google_sub=?`, sub)
	var id int64
	if err := row.Scan(&id); err == nil {
		_, _ = p.DB.Exec(`UPDATE users SET email_verified=1, updated_at=? WHERE id=?`, now, id)
		return p.GetUser(id)
	}

	// Link by email
	row = p.DB.QueryRow(`SELECT id FROM users WHERE lower(email)=?`, email)
	if err := row.Scan(&id); err == nil {
		_, err = p.DB.Exec(`UPDATE users SET google_sub=?, email_verified=1, updated_at=? WHERE id=?`, sub, now, id)
		if err != nil {
			return nil, err
		}
		return p.GetUser(id)
	}

	if !p.Cfg.RegisterEnabled {
		return nil, errors.New("registration disabled")
	}
	login := suggestLogin(email, name)
	base := login
	for i := 0; i < 20; i++ {
		cand := login
		if i > 0 {
			cand = fmt.Sprintf("%s%d", base, i+1)
		}
		var exists int
		_ = p.DB.QueryRow(`SELECT COUNT(1) FROM users WHERE lower(login)=?`, cand).Scan(&exists)
		if exists == 0 {
			login = cand
			break
		}
	}
	if name == "" {
		name = login
	}
	verified := 1
	if !emailVerified {
		verified = 0
	}
	// Google users may have empty password_hash — store placeholder empty bcrypt-incompatible ""
	res, err := p.DB.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, google_sub, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		email, "", p.Cfg.RegisterDefaultRole, login, name, verified, sub, now, now)
	if err != nil {
		return nil, err
	}
	id, _ = res.LastInsertId()
	return p.GetUser(id)
}

func suggestLogin(email, name string) string {
	base := strings.Split(email, "@")[0]
	base = strings.ToLower(base)
	var b strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) < 3 {
		out = "user"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "u" + out
	}
	if len(out) > 28 {
		out = out[:28]
	}
	if !loginRe.MatchString(out) {
		return "user"
	}
	return out
}

func (p *Platform) AuthPublicConfig() map[string]any {
	return map[string]any{
		"register_enabled":     p.Cfg.RegisterEnabled,
		"require_email_verify": p.Cfg.RequireEmailVerify,
		"google_enabled":       p.Cfg.GoogleEnabled(),
		"platform_name":        p.Cfg.PlatformName,
	}
}

// ── Avatar ──────────────────────────────────────────────────────────────────

const maxAvatarBytes = 2 << 20 // 2 MB

var allowedAvatarTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// AvatarDir returns the directory where avatar files are stored.
func (p *Platform) AvatarDir() string {
	return filepath.Join(p.Cfg.DataDir, "avatars")
}

// AvatarFilePath returns the absolute path to a user's avatar file (empty if none).
func (p *Platform) AvatarFilePath(userID int64) string {
	u, err := p.GetUser(userID)
	if err != nil || u.AvatarURL == "" {
		return ""
	}
	// avatar_url is stored as "/api/v1/me/avatar" — file is at data/avatars/{userID}{ext}
	dir := p.AvatarDir()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if len(name) > 0 && strings.HasPrefix(name, strconv.FormatInt(userID, 10)) {
			return filepath.Join(dir, name)
		}
	}
	return ""
}

// SetAvatarFromFile validates and saves an uploaded avatar image for the user.
func (p *Platform) SetAvatarFromFile(userID int64, file io.ReadCloser, contentType, fileName string) (*models.User, error) {
	defer file.Close()

	ext, ok := allowedAvatarTypes[contentType]
	if !ok {
		// Fallback: guess from file extension
		ext = strings.ToLower(filepath.Ext(fileName))
		switch ext {
		case ".jpg", ".jpeg":
			ext = ".jpg"
		case ".png":
			ext = ".png"
		case ".webp":
			ext = ".webp"
		default:
			return nil, errors.New("unsupported image type: use JPEG, PNG or WebP")
		}
	}

	dir := p.AvatarDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create avatar dir: %w", err)
	}

	// Remove old avatar files for this user
	oldEntries, _ := os.ReadDir(dir)
	for _, e := range oldEntries {
		if strings.HasPrefix(e.Name(), strconv.FormatInt(userID, 10)+".") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}

	destName := strconv.FormatInt(userID, 10) + ext
	destPath := filepath.Join(dir, destName)

	dst, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("create avatar file: %w", err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("write avatar: %w", err)
	}
	if written > maxAvatarBytes {
		_ = os.Remove(destPath)
		return nil, errors.New("avatar file too large (max 2 MB)")
	}

	// Update DB
	now := db.NowMS()
	_, err = p.DB.Exec(`UPDATE users SET avatar_url=?, updated_at=? WHERE id=?`, "/api/v1/me/avatar", now, userID)
	if err != nil {
		_ = os.Remove(destPath)
		return nil, err
	}
	return p.GetUser(userID)
}

// DeleteAvatar removes the user's avatar file and clears the DB field.
func (p *Platform) DeleteAvatar(userID int64) error {
	dir := p.AvatarDir()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), strconv.FormatInt(userID, 10)+".") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	now := db.NowMS()
	_, err := p.DB.Exec(`UPDATE users SET avatar_url='', updated_at=? WHERE id=?`, now, userID)
	return err
}

// silence unused import if time only used via db
var _ = time.Hour
