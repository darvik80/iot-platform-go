package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

// SetupState holds the wizard state during the 8-step setup process.
type SetupState struct {
	mu sync.RWMutex

	// Current step (1-8)
	CurrentStep int `json:"current_step"`

	// Step 1: Database
	DatabaseDriver string `json:"database_driver"`
	DatabaseDSN    string `json:"database_dsn"`

	// Step 2: Admin
	AdminLogin    string `json:"admin_login"`
	AdminEmail    string `json:"admin_email"`
	AdminPassword string `json:"admin_password"`

	// Step 3: Account
	AccountSlug string `json:"account_slug"`
	AccountName string `json:"account_name"`

	// Step 4: SMTP
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	SMTPUser string `json:"smtp_user"`
	SMTPPass string `json:"smtp_pass"`
	SMTPFrom string `json:"smtp_from"`

	// Step 5: JWT
	JWTSecret string `json:"jwt_secret"`

	// Step 6: MQTT TLS
	MQTTTLSCert string `json:"mqtt_tls_cert"`
	MQTTTLSKey  string `json:"mqtt_tls_key"`

	// Step 7: Public URL
	PublicBaseURL string `json:"public_base_url"`

	// Error state
	Error string `json:"error,omitempty"`

	// Whether setup is complete
	Completed bool `json:"completed"`
}

// SetupService manages the setup wizard state.
type SetupService struct {
	state *SetupState
	log   *slog.Logger
}

// NewSetupService creates a new setup service.
func NewSetupService(log *slog.Logger) *SetupService {
	return &SetupService{
		state: &SetupState{
			CurrentStep:    1,
			DatabaseDriver: "sqlite",
			DatabaseDSN:    "./data/platform.db",
			AdminLogin:     "admin",
			AdminEmail:     "admin@example.com",
			AccountSlug:    "default",
			AccountName:    "Default",
			SMTPPort:       587,
			JWTSecret:      generateSecret(),
			PublicBaseURL:  "http://localhost:8080",
		},
		log: log,
	}
}

// GetState returns the current setup state.
func (s *SetupService) GetState() SetupState {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	return *s.state
}

// UpdateStep updates the state for a specific step.
func (s *SetupService) UpdateStep(step int, data map[string]any) error {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	if s.state.Completed {
		return errors.New("setup already completed")
	}

	switch step {
	case 1: // Database
		if v, ok := data["database_driver"].(string); ok && v != "" {
			s.state.DatabaseDriver = v
		}
		if v, ok := data["database_dsn"].(string); ok && v != "" {
			s.state.DatabaseDSN = v
		}
	case 2: // Admin
		if v, ok := data["admin_login"].(string); ok && v != "" {
			s.state.AdminLogin = v
		}
		if v, ok := data["admin_email"].(string); ok && v != "" {
			s.state.AdminEmail = v
		}
		if v, ok := data["admin_password"].(string); ok && v != "" {
			s.state.AdminPassword = v
		}
	case 3: // Account
		if v, ok := data["account_slug"].(string); ok && v != "" {
			s.state.AccountSlug = v
		}
		if v, ok := data["account_name"].(string); ok && v != "" {
			s.state.AccountName = v
		}
	case 4: // SMTP
		if v, ok := data["smtp_host"].(string); ok {
			s.state.SMTPHost = v
		}
		if v, ok := data["smtp_port"].(float64); ok {
			s.state.SMTPPort = int(v)
		}
		if v, ok := data["smtp_user"].(string); ok {
			s.state.SMTPUser = v
		}
		if v, ok := data["smtp_pass"].(string); ok {
			s.state.SMTPPass = v
		}
		if v, ok := data["smtp_from"].(string); ok {
			s.state.SMTPFrom = v
		}
	case 5: // JWT
		if v, ok := data["jwt_secret"].(string); ok && v != "" {
			s.state.JWTSecret = v
		}
	case 6: // MQTT TLS
		if v, ok := data["mqtt_tls_cert"].(string); ok {
			s.state.MQTTTLSCert = v
		}
		if v, ok := data["mqtt_tls_key"].(string); ok {
			s.state.MQTTTLSKey = v
		}
	case 7: // Public URL
		if v, ok := data["public_base_url"].(string); ok && v != "" {
			s.state.PublicBaseURL = v
		}
	default:
		return fmt.Errorf("invalid step: %d", step)
	}

	// Advance to next step if not already there
	if step == s.state.CurrentStep && step < 8 {
		s.state.CurrentStep = step + 1
	}

	return nil
}

// Apply executes the setup wizard and creates the initial database.
func (s *SetupService) Apply() error {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	if s.state.Completed {
		return errors.New("setup already completed")
	}

	s.log.Info("applying setup wizard",
		"driver", s.state.DatabaseDriver,
		"admin_email", s.state.AdminEmail,
		"account_slug", s.state.AccountSlug)

	// Build config from wizard state
	cfg := config.Config{
		DatabaseDriver:       s.state.DatabaseDriver,
		DatabaseDSN:          s.state.DatabaseDSN,
		AdminLogin:           s.state.AdminLogin,
		AdminEmail:           s.state.AdminEmail,
		AdminPassword:        s.state.AdminPassword,
		BootstrapAccountSlug: s.state.AccountSlug,
		BootstrapAccountName: s.state.AccountName,
		JWTSecret:            s.state.JWTSecret,
		SMTPHost:             s.state.SMTPHost,
		SMTPPort:             s.state.SMTPPort,
		SMTPUser:             s.state.SMTPUser,
		SMTPPass:             s.state.SMTPPass,
		SMTPFrom:             s.state.SMTPFrom,
		MQTTTLSCert:          s.state.MQTTTLSCert,
		MQTTTLSKey:           s.state.MQTTTLSKey,
		PublicBaseURL:        s.state.PublicBaseURL,
	}

	// Open database and run migrations
	database, err := db.Open(cfg.DatabaseDriver, cfg.DatabaseDSN)
	if err != nil {
		s.state.Error = fmt.Sprintf("database open failed: %v", err)
		return err
	}
	defer database.Close()

	// Check if accounts already exist (setup already done)
	var accountCount int
	if err := database.QueryRow(`SELECT COUNT(1) FROM accounts`).Scan(&accountCount); err == nil && accountCount > 0 {
		s.state.Completed = true
		return errors.New("setup already completed: accounts exist")
	}

	// Create bootstrap admin and default account
	if err := bootstrapSetup(database, cfg); err != nil {
		s.state.Error = fmt.Sprintf("bootstrap failed: %v", err)
		return err
	}

	// Store configuration in settings table
	if err := storeSettings(database, cfg); err != nil {
		s.state.Error = fmt.Sprintf("store settings failed: %v", err)
		return err
	}

	s.state.Completed = true
	s.log.Info("setup wizard completed successfully")
	return nil
}

// bootstrapSetup creates the initial admin user and default account.
func bootstrapSetup(database *db.DB, cfg config.Config) error {
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	now := db.NowMS()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Create admin user (wrapper auto-converts ? → $N and handles RETURNING id for PostgreSQL)
	res, err := tx.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, is_platform_admin, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		cfg.AdminEmail, hash, models.RoleAdmin, cfg.AdminLogin, "Admin", 1, 1, now, now)
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	adminID, _ := res.LastInsertId()

	// Create default account
	accRes, err := tx.Exec(`INSERT INTO accounts(slug, name, settings_json, status, created_at, updated_at)
		VALUES(?,?,?,?,?,?)`,
		cfg.BootstrapAccountSlug, cfg.BootstrapAccountName, "{}", "active", now, now)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	accountID, _ := accRes.LastInsertId()

	// Create membership
	_, err = tx.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at)
		VALUES(?,?,?,?,?)`,
		adminID, accountID, models.RoleAdmin, "active", now)
	if err != nil {
		return fmt.Errorf("create membership: %w", err)
	}

	return tx.Commit()
}

// storeSettings persists wizard configuration to the settings table.
func storeSettings(database *db.DB, cfg config.Config) error {
	settings := map[string]string{
		"jwt_secret":      cfg.JWTSecret,
		"smtp_host":       cfg.SMTPHost,
		"smtp_port":       fmt.Sprintf("%d", cfg.SMTPPort),
		"smtp_user":       cfg.SMTPUser,
		"smtp_pass":       cfg.SMTPPass,
		"smtp_from":       cfg.SMTPFrom,
		"mqtt_tls_cert":   cfg.MQTTTLSCert,
		"mqtt_tls_key":    cfg.MQTTTLSKey,
		"public_base_url": cfg.PublicBaseURL,
	}

	for key, value := range settings {
		if value == "" {
			continue
		}
		_, err := database.Exec(`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
		if err != nil {
			return fmt.Errorf("store setting %s: %w", key, err)
		}
	}
	return nil
}

// generateSecret creates a random 32-byte hex string.
func generateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a fixed secret if crypto/rand fails (should not happen)
		return "dev-jwt-secret-change-me"
	}
	return hex.EncodeToString(b)
}
