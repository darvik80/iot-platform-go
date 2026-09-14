package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

// Apply executes the setup wizard and creates the initial data.
// Opens a connection to the database specified in the wizard state.
func (s *SetupService) Apply() error {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	if s.state.Completed {
		return errors.New("setup already completed")
	}

	s.log.Info("applying setup wizard",
		"driver", s.state.DatabaseDriver,
		"dsn", s.state.DatabaseDSN,
		"admin_email", s.state.AdminEmail,
		"account_slug", s.state.AccountSlug)

	// Open connection to the database specified in the wizard
	database, err := db.Open(s.state.DatabaseDriver, s.state.DatabaseDSN)
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

	// Build config from wizard state
	cfg := config.Config{
		Database: config.DatabaseConfig{
			Driver: s.state.DatabaseDriver,
			DSN:    s.state.DatabaseDSN,
		},
		Admin: config.AdminConfig{
			Login:    s.state.AdminLogin,
			Email:    s.state.AdminEmail,
			Password: s.state.AdminPassword,
		},
		Bootstrap: config.BootstrapConfig{
			AccountSlug: s.state.AccountSlug,
			AccountName: s.state.AccountName,
		},
		Auth: config.AuthConfig{
			JWTSecret: s.state.JWTSecret,
		},
		Email: config.EmailConfig{
			SMTPHost: s.state.SMTPHost,
			SMTPPort: s.state.SMTPPort,
			SMTPUser: s.state.SMTPUser,
			SMTPPass: s.state.SMTPPass,
			SMTPFrom: s.state.SMTPFrom,
		},
		MQTT: config.MQTTConfig{
			TLSCert: s.state.MQTTTLSCert,
			TLSKey:  s.state.MQTTTLSKey,
		},
		Platform: config.PlatformConfig{
			PublicBaseURL: s.state.PublicBaseURL,
		},
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

	// Write configuration to config.yml for persistence
	if err := writeConfigYAML(cfg); err != nil {
		s.log.Warn("failed to write config.yml", "err", err)
		// Not fatal - settings are already in the database
	}

	s.state.Completed = true
	s.log.Info("setup wizard completed successfully")
	return nil
}

// bootstrapSetup creates the initial admin user and default account.
func bootstrapSetup(database *db.DB, cfg config.Config) error {
	hash, err := auth.HashPassword(cfg.Admin.Password)
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
	res, err := tx.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		cfg.Admin.Email, hash, models.RoleAdmin, cfg.Admin.Login, "Admin", 1, now, now)
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	adminID, _ := res.LastInsertId()

	// Create default account
	accRes, err := tx.Exec(`INSERT INTO accounts(slug, name, settings_json, status, created_at, updated_at)
		VALUES(?,?,?,?,?,?)`,
		cfg.Bootstrap.AccountSlug, cfg.Bootstrap.AccountName, "{}", "active", now, now)
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
		"jwt_secret":      cfg.Auth.JWTSecret,
		"smtp_host":       cfg.Email.SMTPHost,
		"smtp_port":       fmt.Sprintf("%d", cfg.Email.SMTPPort),
		"smtp_user":       cfg.Email.SMTPUser,
		"smtp_pass":       cfg.Email.SMTPPass,
		"smtp_from":       cfg.Email.SMTPFrom,
		"mqtt_tls_cert":   cfg.MQTT.TLSCert,
		"mqtt_tls_key":    cfg.MQTT.TLSKey,
		"public_base_url": cfg.Platform.PublicBaseURL,
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

// writeConfigYAML writes the configuration to config.yml for persistence.
func writeConfigYAML(cfg config.Config) error {
	// Build a minimal config structure for YAML
	data := map[string]any{
		"database_driver": cfg.Database.Driver,
		"database_dsn":    cfg.Database.DSN,
		"jwt_secret":      cfg.Auth.JWTSecret,
		"public_base_url": cfg.Platform.PublicBaseURL,
	}
	if cfg.Email.SMTPHost != "" {
		data["smtp_host"] = cfg.Email.SMTPHost
		data["smtp_port"] = cfg.Email.SMTPPort
		data["smtp_user"] = cfg.Email.SMTPUser
		data["smtp_pass"] = cfg.Email.SMTPPass
		data["smtp_from"] = cfg.Email.SMTPFrom
	}
	if cfg.MQTT.TLSCert != "" {
		data["mqtt_tls_cert"] = cfg.MQTT.TLSCert
		data["mqtt_tls_key"] = cfg.MQTT.TLSKey
	}

	// Marshal to YAML
	yamlData, err := marshalYAML(data)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}

	// Write to config.yml
	if err := os.WriteFile("config.yml", yamlData, 0644); err != nil {
		return fmt.Errorf("write config.yml: %w", err)
	}
	return nil
}

// marshalYAML is a simple YAML marshaller for map[string]any.
// This is a simplified implementation; for production use, consider using a proper YAML library.
func marshalYAML(data map[string]any) ([]byte, error) {
	var result []byte
	for key, value := range data {
		switch v := value.(type) {
		case string:
			result = append(result, fmt.Sprintf("%s: %q\n", key, v)...)
		case int:
			result = append(result, fmt.Sprintf("%s: %d\n", key, v)...)
		default:
			result = append(result, fmt.Sprintf("%s: %v\n", key, v)...)
		}
	}
	return result, nil
}
