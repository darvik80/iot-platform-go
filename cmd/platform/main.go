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

	"crearts-iot-platform/internal/api"
	"crearts-iot-platform/internal/broker"
	"crearts-iot-platform/internal/config"
	"crearts-iot-platform/internal/db"
	mqttclient "crearts-iot-platform/internal/mqtt"
	"crearts-iot-platform/internal/services"
	"crearts-iot-platform/internal/webui"
	"crearts-iot-platform/internal/ws"
)

func main() {
	config.LoadDotEnv(".env")
	config.LoadYAML("./config.yml")
	config.LoadYAML("/etc/iot-platform-go/config.yml")
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
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

	br := broker.New(cfg.RabbitMQMgmtURL, cfg.MQTTUsername, cfg.MQTTPassword)
	plat := services.New(database, cfg, br, log)
	if err := plat.EnsureAdmin(); err != nil {
		log.Error("ensure admin failed", "err", err)
		os.Exit(1)
	}
	_ = plat.ReloadDeviceMaps()

	hub := ws.NewHub()
	plat.SetHub(hub)

	mqtt := mqttclient.New(cfg, plat, log)
	plat.SetPublisher(mqtt)
	if err := mqtt.Start(); err != nil {
		log.Warn("broker ingest not ready (will keep serving REST)", "err", err)
	}
	defer mqtt.Close()

	noderedPass := os.Getenv("NODERED_MQTT_PASSWORD")
	if noderedPass == "" {
		noderedPass = "nodered"
	}
	if err := br.Ping(); err != nil {
		log.Warn("rabbitmq management not ready — device create will fail until it is", "url", cfg.RabbitMQMgmtURL, "err", err)
	} else {
		log.Info("rabbitmq management ok", "url", cfg.RabbitMQMgmtURL)
		if err := br.EnsureNodeRedUser("nodered", noderedPass); err != nil {
			log.Warn("nodered user provision skipped", "err", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go plat.RetentionLoop(ctx)

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
		log.Info("platform listening", "addr", cfg.HTTPAddr, "name", cfg.PlatformName)
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
