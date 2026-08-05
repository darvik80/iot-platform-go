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
	"regexp"
	"strings"
	"time"

	"crearts-iot-platform/internal/auth"
	"crearts-iot-platform/internal/db"
	"crearts-iot-platform/internal/mail"
	"crearts-iot-platform/internal/models"
)

var loginRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`)

func (p *Platform) Mailer() *mail.Sender {
	return &mail.Sender{Cfg: p.Cfg, Log: p.Log}
}

func (p *Platform) EnsureAdmin() error {
	var n int
	if err := p.DB.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		// Ensure migration columns filled for bootstrap admin if needed.
		_, _ = p.DB.Exec(`UPDATE users SET login=? WHERE (login IS NULL OR login='') AND email=?`,
			p.Cfg.AdminLogin, p.Cfg.AdminEmail)
		_, _ = p.DB.Exec(`UPDATE users SET email_verified=1 WHERE email=?`, p.Cfg.AdminEmail)
		return nil
	}
	hash, err := auth.HashPassword(p.Cfg.AdminPassword)
	if err != nil {
		return err
	}
	now := db.NowMS()
	_, err = p.DB.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		p.Cfg.AdminEmail, hash, models.RoleAdmin, p.Cfg.AdminLogin, "Admin", 1, now, now)
	return err
}

type RegisterInput struct {
	Login       string `json:"login"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
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
	res, err := p.DB.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		in.Email, hash, p.Cfg.RegisterDefaultRole, in.Login, in.DisplayName, verified, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("login or email already registered")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	u, err := p.GetUser(id)
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
	tok, err := auth.IssueJWT(p.Cfg.JWTSecret, *u, 24*time.Hour)
	if err != nil {
		return nil, "", err
	}
	return u, tok, nil
}

func (p *Platform) FindUserByLoginOrEmail(identity string) (*models.User, error) {
	identity = strings.ToLower(strings.TrimSpace(identity))
	row := p.DB.QueryRow(`SELECT id, COALESCE(login,''), email, COALESCE(display_name,''), password_hash, role,
		COALESCE(email_verified,0), COALESCE(google_sub,''), COALESCE(disabled,0), created_at, updated_at
		FROM users WHERE lower(email)=? OR lower(login)=?`, identity, identity)
	return scanUser(row)
}

func (p *Platform) GetUser(id int64) (*models.User, error) {
	row := p.DB.QueryRow(`SELECT id, COALESCE(login,''), email, COALESCE(display_name,''), password_hash, role,
		COALESCE(email_verified,0), COALESCE(google_sub,''), COALESCE(disabled,0), created_at, updated_at
		FROM users WHERE id=?`, id)
	return scanUser(row)
}

func (p *Platform) ListUsers() ([]models.User, error) {
	rows, err := p.DB.Query(`SELECT id, COALESCE(login,''), email, COALESCE(display_name,''), password_hash, role,
		COALESCE(email_verified,0), COALESCE(google_sub,''), COALESCE(disabled,0), created_at, updated_at
		FROM users ORDER BY id`)
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
	dis := 0
	if u.Disabled {
		dis = 1
	}
	_, err = p.DB.Exec(`UPDATE users SET role=?, disabled=?, display_name=?, updated_at=? WHERE id=?`,
		u.Role, dis, u.DisplayName, db.NowMS(), id)
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

func (p *Platform) UserByAPIToken(token string) (*models.User, error) {
	h := auth.HashToken(token)
	var id int64
	err := p.DB.QueryRow(`SELECT user_id FROM api_tokens WHERE token_hash=?`, h).Scan(&id)
	if err != nil {
		return nil, err
	}
	_, _ = p.DB.Exec(`UPDATE api_tokens SET last_used_at=? WHERE token_hash=?`, db.NowMS(), h)
	return p.GetUser(id)
}

func (p *Platform) ListAPITokens(userID int64) ([]models.APIToken, error) {
	rows, err := p.DB.Query(`SELECT id, user_id, name, token_prefix, created_at, last_used_at
		FROM api_tokens WHERE user_id=? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.APIToken
	for rows.Next() {
		var t models.APIToken
		var last sql.NullInt64
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.CreatedAt, &last); err != nil {
			return nil, err
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

func (p *Platform) CreateAPIToken(userID int64, name string) (*models.APIToken, error) {
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
	res, err := p.DB.Exec(`INSERT INTO api_tokens(user_id, name, token_hash, token_prefix, created_at) VALUES(?,?,?,?,?)`,
		userID, name, auth.HashToken(raw), prefix, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.APIToken{
		ID:        id,
		UserID:    userID,
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

func scanUser(row interface{ Scan(dest ...any) error }) (*models.User, error) {
	var u models.User
	var verified, disabled int
	err := row.Scan(&u.ID, &u.Login, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Role,
		&verified, &u.GoogleSub, &disabled, &u.CreatedAt, &u.UpdatedAt)
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
	jwt, err := auth.IssueJWT(p.Cfg.JWTSecret, *u, 24*time.Hour)
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

// silence unused import if time only used via db
var _ = time.Hour
