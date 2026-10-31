package metrics

import (
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Start launches the scrape listener in the background. The caller owns
// shutdown; a failed bind is logged and retried by systemd/docker restart.
func Start(addr string, log *slog.Logger) *http.Server {
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Info("metrics listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics listener failed", "addr", addr, "err", err)
		}
	}()
	return srv
}
