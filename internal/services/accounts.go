package services

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// CreateAccount creates a new tenant account and optionally adds the creating user as admin.
func (p *Platform) CreateAccount(slug, name string, creatorUserID int64) (*models.Account, error) {
	if slug == "" || name == "" {
		return nil, errors.New("slug and name are required")
	}
	if !slugRe.MatchString(slug) {
		return nil, errors.New("slug must start with lowercase letter/digit and contain only lowercase letters, digits, dots, hyphens, underscores")
	}
	if len(slug) > 64 {
		return nil, errors.New("slug too long (max 64 chars)")
	}
	if len(name) > 128 {
		return nil, errors.New("name too long (max 128 chars)")
	}

	now := db.NowMS()

	tx, err := p.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check slug uniqueness
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM accounts WHERE slug=?`, slug).Scan(&exists); err == nil && exists > 0 {
		return nil, fmt.Errorf("slug %q already taken", slug)
	}

	res, err := tx.Exec(`INSERT INTO accounts(slug, name, settings_json, status, created_at, updated_at)
		VALUES(?,?,?,'active',?,?)`,
		slug, name, "{}", now, now)
	if err != nil {
		return nil, fmt.Errorf("insert account: %w", err)
	}
	accountID, _ := res.LastInsertId()

	// Add creator as admin if provided
	if creatorUserID > 0 {
		if _, err := tx.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at)
			VALUES(?,?,?,'active',?)`,
			creatorUserID, accountID, models.RoleAdmin, now); err != nil {
			return nil, fmt.Errorf("add creator membership: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	acc := &models.Account{
		ID:           accountID,
		Slug:         slug,
		Name:         name,
		SettingsJSON: "{}",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if p.Log != nil {
		p.Log.Info("account created", "account_id", accountID, "slug", slug, "name", name, "creator_user_id", creatorUserID)
	}

	return acc, nil
}

// DeleteAccount permanently removes an account and all its data.
// If force is false, the account must have zero devices and zero external plugins.
// If force is true, all products, devices, plugins and related data are cascade-deleted.
func (p *Platform) DeleteAccount(accountID int64, force bool) error {
	if accountID <= 0 {
		return errors.New("invalid account id")
	}

	// Verify account exists
	var slug string
	err := p.DB.QueryRow(`SELECT slug FROM accounts WHERE id=?`, accountID).Scan(&slug)
	if err == sql.ErrNoRows {
		return errors.New("account not found")
	}
	if err != nil {
		return err
	}

	// Count devices and external plugins
	var deviceCount, pluginCount int
	_ = p.DB.QueryRow(`SELECT COUNT(*) FROM products pr JOIN devices d ON d.product_pk=pr.id WHERE pr.account_id=?`, accountID).Scan(&deviceCount)
	_ = p.DB.QueryRow(`SELECT COUNT(*) FROM external_plugins WHERE account_id=?`, accountID).Scan(&pluginCount)

	if !force && (deviceCount > 0 || pluginCount > 0) {
		return fmt.Errorf("account has %d devices and %d external plugins; use force=true to delete", deviceCount, pluginCount)
	}

	tx, err := p.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := db.NowMS()

	// Delete external plugins for this account
	if _, err := tx.Exec(`DELETE FROM external_plugins WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete external plugins: %w", err)
	}

	// Delete products (cascades to devices, which cascades to telemetry, events, logs, rpc, bindings, etc.)
	// First get all product PKs to clean up device maps
	rows, err := tx.Query(`SELECT id FROM products WHERE account_id=?`, accountID)
	if err != nil {
		return err
	}
	var productPKs []int64
	for rows.Next() {
		var pk int64
		if err := rows.Scan(&pk); err != nil {
			rows.Close()
			return err
		}
		productPKs = append(productPKs, pk)
	}
	rows.Close()

	// Delete firmwares (cascades from products)
	if _, err := tx.Exec(`DELETE FROM ota_firmwares WHERE product_pk IN (SELECT id FROM products WHERE account_id=?)`, accountID); err != nil {
		return fmt.Errorf("delete firmwares: %w", err)
	}

	// Delete products (cascades to devices, then to all device-scoped tables)
	if _, err := tx.Exec(`DELETE FROM products WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete products: %w", err)
	}

	// Delete account-scoped data that may not cascade
	if _, err := tx.Exec(`DELETE FROM events WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete events: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM device_logs WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete logs: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM telemetry_samples WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete telemetry: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM rpc_calls WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete rpc: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM ota_jobs WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete ota jobs: %w", err)
	}

	// Delete memberships, invitations, plugin policies (should cascade from account)
	if _, err := tx.Exec(`DELETE FROM account_memberships WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete memberships: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM account_plugin_policies WHERE account_id=?`, accountID); err != nil {
		return fmt.Errorf("delete plugin policies: %w", err)
	}

	// Delete invitations
	if _, err := tx.Exec(`DELETE FROM account_invitations WHERE account_id=?`, accountID); err != nil {
		// Table may not exist in all schemas
		_ = err
	}

	// Mark audit log entries with a note (preserve for 30 days per spec)
	if _, err := tx.Exec(`UPDATE audit_log SET detail_json=json_patch(COALESCE(detail_json,'{}'), '{"account_deleted":true}') WHERE account_id=?`, accountID); err != nil {
		// account_id may not be on audit_log in all schemas
		_ = err
	}

	// Finally delete the account
	if _, err := tx.Exec(`DELETE FROM accounts WHERE id=?`, accountID); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Reload device maps (removes deleted devices)
	_ = p.ReloadDeviceMaps()

	// Log the deletion
	if p.Log != nil {
		p.Log.Info("account deleted", "account_id", accountID, "slug", slug, "devices", deviceCount, "plugins", pluginCount, "ts", now)
	}

	return nil
}
