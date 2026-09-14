package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

func main() {
	// Load configuration from YAML only (no .env)
	config.LoadYAML("./config.yml")
	config.LoadYAML("/etc/iot-platform-go/config.yml")
	cfg := config.Load()

	// Use defaults if not configured
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = ":8080"
	}
	if cfg.MQTT.Addr == "" {
		cfg.MQTT.Addr = ":1883"
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.SlogLevel()}))
	slog.SetDefault(log)
	if cfg.GoogleEnabled() {
		log.Info("google oauth enabled", "redirect", cfg.GoogleCallbackURL())
	} else {
		log.Info("google oauth disabled — configure via setup wizard or config.yml")
	}

	// Check if we need to run setup wizard
	// Setup is needed if database is not configured or the configured database is empty
	var database *db.DB
	var setupService *services.SetupService
	var plat *services.Platform
	needSetup := false

	if cfg.Database.Driver != "" && cfg.Database.DSN != "" {
		// Database is configured — try to open it
		var err error
		database, err = db.Open(cfg.Database.Driver, cfg.Database.DSN)
		if err != nil {
			log.Error("database open failed", "driver", cfg.Database.Driver, "err", err)
			os.Exit(1)
		}

		// Check if database has data
		var userCount, accountCount int
		if err := database.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&userCount); err != nil {
			userCount = 0
		}
		if err := database.QueryRow(`SELECT COUNT(1) FROM accounts`).Scan(&accountCount); err != nil {
			accountCount = 0
		}

		if userCount == 0 && accountCount == 0 {
			// Database is empty — need setup
			log.Info("empty database detected, enabling setup wizard")
			needSetup = true
			// Close the empty database — wizard will open the configured one
			database.Close()
			database = nil
		} else {
			// Database has data — normal startup
			plat = services.New(database, cfg, log)
			if err := plat.EnsureAdmin(); err != nil {
				log.Error("ensure admin failed", "err", err)
				os.Exit(1)
			}
		}
	} else {
		// No database configured — need setup
		log.Info("no database configured, enabling setup wizard")
		needSetup = true
	}

	if needSetup {
		// Start HTTP server with setup wizard only
		setupService = services.NewSetupService(log)

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

		// Wait for setup to complete
		for {
			time.Sleep(1 * time.Second)
			state := setupService.GetState()
			if state.Completed {
				break
			}
		}

		_ = httpServer.Shutdown(context.Background())

		// Reload config (wizard wrote config.yml) — force override any existing env vars
		config.LoadYAMLForce("./config.yml")
		cfg = config.Load()
		if cfg.HTTP.Addr == "" {
			cfg.HTTP.Addr = ":8080"
		}
		if cfg.MQTT.Addr == "" {
			cfg.MQTT.Addr = ":1883"
		}

		// Open the database configured by the wizard
		var err error
		database, err = db.Open(cfg.Database.Driver, cfg.Database.DSN)
		if err != nil {
			log.Error("database open failed after setup", "err", err)
			os.Exit(1)
		}
		plat = services.New(database, cfg, log)
	}

	defer database.Close()
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
	defer brk.Close()

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

	srv := &api.Server{Plat: plat, Hub: hub, SpaFS: spa, Setup: setupService}
	httpServer := &http.Server{Addr: cfg.HTTP.Addr, Handler: srv.Router()}

	go func() {
		log.Info("platform listening", "addr", cfg.HTTP.Addr, "mqtt", cfg.MQTT.Addr, "public", cfg.Platform.PublicBaseURL, "name", cfg.Platform.Name, "tz", cfg.TimezoneOrUTC(), "log", cfg.LogLevel)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
	shutdownCtx, c2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer c2()
	_ = httpServer.Shutdown(shutdownCtx)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
