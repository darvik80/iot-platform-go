package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"cogitor-iot-platform/internal/api"
	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	mqttbroker "cogitor-iot-platform/internal/mqtt"
	"cogitor-iot-platform/internal/services"
	"cogitor-iot-platform/internal/webui"
	"cogitor-iot-platform/internal/ws"
)

// Build-time injection via -ldflags.
var (
	Version = "dev"
	Commit  = "unknown"
)

func main() {
	cfg, log := initConfig()

	database, plat, setupService := initDatabase(cfg, log)
	defer database.Close()

	startPlatform(cfg, log, database, plat, setupService)
}

// initConfig loads YAML configs, builds Config and creates the logger.
func initConfig() (config.Config, *slog.Logger) {
	loaded, errs := config.LoadFiles(false)
	cfg := config.Load()

	var log *slog.Logger
	opts := &slog.HandlerOptions{Level: cfg.SlogLevel()}
	if cfg.LogFormat == "json" {
		log = slog.New(slog.NewJSONHandler(os.Stdout, opts))
	} else {
		log = slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	slog.SetDefault(log)

	for _, err := range errs {
		log.Error("config file ignored", "err", err)
	}

	log.Info("config sources",
		"files", configSources(loaded),
		"db_driver", cfg.Database.Driver,
		"db_dsn", config.RedactDSN(cfg.Database.DSN),
		"http_addr", cfg.HTTP.Addr,
		"mqtt_addr", cfg.MQTT.Addr,
		"data_dir", cfg.DataDir,
		"email_mode", cfg.Email.EffectiveMode(),
		"smtp_host", cfg.Email.SMTPHost,
		"version", Version,
		"commit", Commit,
	)
	if cfg.GoogleEnabled() {
		log.Info("google oauth enabled", "redirect", cfg.GoogleCallbackURL())
	} else {
		log.Info("google oauth disabled — configure via setup wizard or config.yml")
	}
	return cfg, log
}

// initDatabase opens the database or runs the setup wizard if needed.
// Returns the database, platform service, and optionally the setup service.
func initDatabase(cfg config.Config, log *slog.Logger) (*db.DB, *services.Platform, *services.SetupService) {
	if cfg.Database.Driver != "" && cfg.Database.DSN != "" {
		database, err := db.Open(cfg.Database.Driver, cfg.Database.DSN)
		if err != nil {
			log.Error("database open failed", "driver", cfg.Database.Driver, "err", err)
			os.Exit(1)
		}

		if dbIsEmpty(database) {
			database.Close()
			log.Info("empty database detected, enabling setup wizard")
			return runSetupWizard(cfg, log)
		}

		ensureJWTSecret(database, &cfg, log)
		plat := services.New(database, cfg, log)
		if err := plat.EnsureAdmin(); err != nil {
			log.Error("ensure admin failed", "err", err)
			os.Exit(1)
		}
		return database, plat, nil
	}

	log.Info("no database configured, enabling setup wizard")
	return runSetupWizard(cfg, log)
}

// dbIsEmpty returns true if the database has no users and no accounts.
func dbIsEmpty(database *db.DB) bool {
	var userCount, accountCount int
	if err := database.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&userCount); err != nil {
		userCount = 0
	}
	if err := database.QueryRow(`SELECT COUNT(1) FROM accounts`).Scan(&accountCount); err != nil {
		accountCount = 0
	}
	return userCount == 0 && accountCount == 0
}

// runSetupWizard starts the setup-only HTTP server, waits for completion,
// reloads config and opens the database configured by the wizard.
func runSetupWizard(cfg config.Config, log *slog.Logger) (*db.DB, *services.Platform, *services.SetupService) {
	setupService := services.NewSetupService(log)
	srv, httpServer := startSetupHTTP(cfg, setupService, log)

	waitForSetup(setupService)
	_ = httpServer.Shutdown(context.Background())

	// Reload config (wizard wrote config.yml) — force override any existing env vars
	loaded, errs := config.LoadFiles(true)
	for _, err := range errs {
		log.Error("config file ignored", "err", err)
	}
	cfg = config.Load()
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = ":8080"
	}
	if cfg.MQTT.Addr == "" {
		cfg.MQTT.Addr = ":1883"
	}

	log.Info("config reloaded after setup",
		"files", configSources(loaded),
		"db_driver", cfg.Database.Driver,
		"db_dsn", config.RedactDSN(cfg.Database.DSN),
		"email_mode", cfg.Email.EffectiveMode(),
		"smtp_host", cfg.Email.SMTPHost,
	)

	database, err := db.Open(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		log.Error("database open failed after setup", "err", err)
		os.Exit(1)
	}
	ensureJWTSecret(database, &cfg, log)
	plat := services.New(database, cfg, log)
	_ = srv // setup server already shut down
	return database, plat, setupService
}

// ensureJWTSecret refuses to sign tokens with the publicly known dev secret.
func ensureJWTSecret(database *db.DB, cfg *config.Config, log *slog.Logger) {
	src, err := services.EnsureJWTSecret(database, cfg)
	if err != nil {
		log.Error("jwt secret unavailable", "err", err)
		os.Exit(1)
	}
	switch src {
	case "settings":
		log.Warn("JWT_SECRET not configured — using the secret stored by the setup wizard in the database")
	case "generated":
		log.Warn("JWT_SECRET not configured — generated a random secret and stored it in the database; set JWT_SECRET to manage it explicitly")
	}
}

// startSetupHTTP creates and starts the setup-only HTTP server.
func startSetupHTTP(cfg config.Config, setupService *services.SetupService, log *slog.Logger) (*api.Server, *http.Server) {
	var spa fs.FS
	if embedded, err := webui.FS(); err == nil {
		spa = embedded
	} else if disk := os.DirFS(filepath.Clean("web/dist")); fileExists("web/dist/index.html") {
		spa = disk
	}

	srv := &api.Server{Setup: setupService, SpaFS: spa}
	httpServer := &http.Server{Addr: cfg.HTTP.Addr, Handler: srv.Router()}

	log.Info("setup wizard listening", "addr", cfg.HTTP.Addr)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()
	return srv, httpServer
}

// waitForSetup polls the setup service until it reports completion.
func waitForSetup(s *services.SetupService) {
	for {
		time.Sleep(1 * time.Second)
		if s.Completed() {
			break
		}
	}
}

// startPlatform boots the MQTT broker, plugins, retention loop and HTTP server,
// then blocks until a shutdown signal is received.
func startPlatform(cfg config.Config, log *slog.Logger, database *db.DB, plat *services.Platform, setupService *services.SetupService) {
	_ = plat.ReloadDeviceMaps()

	hub := ws.NewHub()
	plat.SetHub(hub)

	brk := mqttbroker.New(cfg, plat, log)
	plat.SetPublisher(brk)
	plat.SetMQTTSession(brk)
	if err := brk.Start(); err != nil {
		log.Error("mqtt broker failed", "addr", cfg.MQTT.Addr, "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go plat.RetentionLoop(ctx)
	plat.StartPlugins(ctx)

	var spa fs.FS
	if embedded, err := webui.FS(); err == nil {
		spa = embedded
		log.Info("serving embedded dashboard")
	} else if disk := os.DirFS(filepath.Clean("web/dist")); fileExists("web/dist/index.html") {
		spa = disk
		log.Info("serving dashboard from web/dist")
	}

	if u, err := url.Parse(cfg.Platform.PublicBaseURL); err == nil && u.Host != "" {
		hub.AllowedHosts = append(hub.AllowedHosts, u.Host)
	}
	srv := &api.Server{Plat: plat, Hub: hub, SpaFS: spa, Setup: setupService, Version: Version, Commit: Commit}
	httpServer := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      srv.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info("platform listening",
			"addr", cfg.HTTP.Addr,
			"mqtt", cfg.MQTT.Addr,
			"public", cfg.Platform.PublicBaseURL,
			"name", cfg.Platform.Name,
			"tz", cfg.TimezoneOrUTC(),
			"log", cfg.LogLevel,
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()

	waitForShutdown()
	log.Info("shutting down...")
	shutdownCtx, c2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer c2()
	
	// Stop background workers
	cancel()
	
	// Shutdown HTTP server (stops accepting new requests, waits for in-flight)
	_ = httpServer.Shutdown(shutdownCtx)
	
	// Close MQTT broker (disconnects all MQTT clients)
	brk.Close()
	
	log.Info("shutdown complete")
}

// waitForShutdown blocks until SIGINT or SIGTERM is received.
func waitForShutdown() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

// configSources renders the loaded config files for the startup log.
func configSources(loaded []string) string {
	if len(loaded) == 0 {
		return "none (env only; looked for " + strings.Join(config.Files, ", ") + ")"
	}
	return strings.Join(loaded, ", ")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
