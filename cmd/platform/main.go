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
	config.LoadDotEnv(".env")
	config.LoadYAML("./config.yml")
	config.LoadYAML("/etc/iot-platform-go/config.yml")
	cfg := config.Load()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.SlogLevel()}))
	slog.SetDefault(log)
	if cfg.GoogleEnabled() {
		log.Info("google oauth enabled", "redirect", cfg.GoogleCallbackURL())
	} else {
		log.Info("google oauth disabled — set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET in .env")
	}

	database, err := db.Open(cfg.SQLitePath)
	if err != nil {
		log.Error("sqlite open failed", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	plat := services.New(database, cfg, log)
	if err := plat.EnsureAdmin(); err != nil {
		log.Error("ensure admin failed", "err", err)
		os.Exit(1)
	}
	_ = plat.ReloadDeviceMaps()

	hub := ws.NewHub()
	plat.SetHub(hub)

	brk := mqttbroker.New(cfg, plat, log)
	plat.SetPublisher(brk)
	plat.SetMQTTSession(brk)
	if err := brk.Start(); err != nil {
		log.Error("mqtt broker failed", "addr", cfg.MQTTAddr, "err", err)
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

	srv := &api.Server{Plat: plat, Hub: hub, SpaFS: spa}
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Router()}

	go func() {
		log.Info("platform listening", "addr", cfg.HTTPAddr, "mqtt", cfg.MQTTAddr, "public", cfg.PublicBaseURL, "name", cfg.PlatformName, "tz", cfg.TimezoneOrUTC(), "log", cfg.LogLevel)
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
