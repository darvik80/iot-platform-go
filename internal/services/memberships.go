package services

import (
	"errors"
	"fmt"
	"time"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/models"
)

// ListUserMemberships returns all active memberships for a user, enriched with account slug/name.
func (p *Platform) ListUserMemberships(userID int64) ([]models.AccountMembershipWithAccount, error) {
	rows, err := p.DB.Query(`SELECT m.user_id, m.account_id, m.role, m.status, m.joined_at, m.invited_by,
		a.slug, a.name, a.status
		FROM account_memberships m
		JOIN accounts a ON a.id = m.account_id
		WHERE m.user_id = ?
		ORDER BY m.joined_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AccountMembershipWithAccount
	for rows.Next() {
		var m models.AccountMembershipWithAccount
		var invitedBy *int64
		if err := rows.Scan(&m.UserID, &m.AccountID, &m.Role, &m.Status, &m.JoinedAt, &invitedBy,
			&m.AccountSlug, &m.AccountName, &m.AccountStatus); err != nil {
			return nil, err
		}
		m.InvitedBy = invitedBy
		out = append(out, m)
	}
	if out == nil {
		out = []models.AccountMembershipWithAccount{}
	}
	return out, rows.Err()
}

// GetMembership returns a single membership.
func (p *Platform) GetMembership(userID, accountID int64) (*models.AccountMembership, error) {
	row := p.DB.QueryRow(`SELECT user_id, account_id, role, status, joined_at, invited_by
		FROM account_memberships WHERE user_id=? AND account_id=?`, userID, accountID)
	var m models.AccountMembership
	if err := row.Scan(&m.UserID, &m.AccountID, &m.Role, &m.Status, &m.JoinedAt, &m.InvitedBy); err != nil {
		return nil, err
	}
	return &m, nil
}

// SwitchAccount validates that the user has an active membership in newAccountID,
// then issues a new JWT with that account as active.
// Returns the new JWT token.
func (p *Platform) SwitchAccount(userID int64, newAccountID int64) (string, error) {
	m, err := p.GetMembership(userID, newAccountID)
	if err != nil {
		return "", errors.New("no membership in that account")
	}
	if m.Status != "active" {
		return "", errors.New("membership not active")
	}
	// Verify account not suspended
	var accStatus string
	if err := p.DB.QueryRow(`SELECT status FROM accounts WHERE id=?`, newAccountID).Scan(&accStatus); err != nil {
		return "", errors.New("account not found")
	}
	if accStatus == "suspended" {
		return "", errors.New("account suspended")
	}

	// Load all refs
	refs, _, err := p.LoadAccountRefs(userID)
	if err != nil {
		return "", err
	}
	// Find user for JWT
	u, err := p.GetUser(userID)
	if err != nil {
		return "", err
	}
	return auth.IssueJWT(p.Cfg.JWTSecret, *u, refs, newAccountID, 24*time.Hour)
}

// ListAccountMembers returns all members of an account with user info.
func (p *Platform) ListAccountMembers(accountID int64) ([]models.MemberInfo, error) {
	rows, err := p.DB.Query(`SELECT m.user_id, COALESCE(u.login,''), u.email, COALESCE(u.display_name,''),
		m.role, m.status, m.joined_at
		FROM account_memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.account_id = ?
		ORDER BY m.joined_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.MemberInfo
	for rows.Next() {
		var m models.MemberInfo
		if err := rows.Scan(&m.UserID, &m.Login, &m.Email, &m.DisplayName, &m.Role, &m.Status, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if out == nil {
		out = []models.MemberInfo{}
	}
	return out, rows.Err()
}

// UpdateMemberRole changes a member's role, with last-admin protection.
func (p *Platform) UpdateMemberRole(accountID, userID int64, newRole models.Role) error {
	switch newRole {
	case models.RoleAdmin, models.RoleOperator, models.RoleViewer:
	default:
		return errors.New("invalid role")
	}
	// Get current role
	m, err := p.GetMembership(userID, accountID)
	if err != nil {
		return errors.New("member not found")
	}
	// Last-admin check: if demoting an admin, ensure another admin exists
	if m.Role == models.RoleAdmin && newRole != models.RoleAdmin {
		if err := p.ensureOtherAdmin(accountID, userID); err != nil {
			return err
		}
	}
	_, err = p.DB.Exec(`UPDATE account_memberships SET role=? WHERE user_id=? AND account_id=?`,
		newRole, userID, accountID)
	return err
}

// RemoveMember removes a user from an account, with last-admin protection.
func (p *Platform) RemoveMember(accountID, userID int64) error {
	m, err := p.GetMembership(userID, accountID)
	if err != nil {
		return errors.New("member not found")
	}
	if m.Role == models.RoleAdmin {
		if err := p.ensureOtherAdmin(accountID, userID); err != nil {
			return err
		}
	}
	_, err = p.DB.Exec(`DELETE FROM account_memberships WHERE user_id=? AND account_id=?`, userID, accountID)
	return err
}

// ensureOtherAdmin returns error if removing/demoting target would leave 0 admins.
func (p *Platform) ensureOtherAdmin(accountID, excludeUserID int64) error {
	var n int
	err := p.DB.QueryRow(`SELECT COUNT(1) FROM account_memberships
		WHERE account_id=? AND role='admin' AND user_id!=?`, accountID, excludeUserID).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("cannot remove last admin from account")
	}
	return nil
}

// GetAccount returns an account if the user is a member (or platform admin).
func (p *Platform) GetAccount(userID, accountID int64) (*models.Account, error) {
	var acc models.Account
	err := p.DB.QueryRow(`SELECT id, slug, name, settings_json, status, created_at, updated_at
		FROM accounts WHERE id=?`, accountID).Scan(
		&acc.ID, &acc.Slug, &acc.Name, &acc.SettingsJSON, &acc.Status, &acc.CreatedAt, &acc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// ListUserAccounts returns accounts the user is a member of.
func (p *Platform) ListUserAccounts(userID int64) ([]models.Account, error) {
	rows, err := p.DB.Query(`SELECT a.id, a.slug, a.name, a.settings_json, a.status, a.created_at, a.updated_at
		FROM accounts a
		JOIN account_memberships m ON m.account_id = a.id
		WHERE m.user_id = ?
		ORDER BY a.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Account
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.Slug, &a.Name, &a.SettingsJSON, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if out == nil {
		out = []models.Account{}
	}
	return out, rows.Err()
}

// AccountSlugByID resolves account slug from ID.
func (p *Platform) AccountSlugByID(accountID int64) (string, error) {
	var slug string
	err := p.DB.QueryRow(`SELECT slug FROM accounts WHERE id=?`, accountID).Scan(&slug)
	if err != nil {
		return "", fmt.Errorf("account slug lookup: %w", err)
	}
	return slug, nil
}
