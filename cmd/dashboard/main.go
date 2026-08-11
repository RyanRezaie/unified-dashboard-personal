// Command dashboard serves the unified semester/homelab dashboard.
//
// LAN/Tailscale only, no login — reachability is an ACL concern, not this
// program's (see the "no auth system" non-goal in CLAUDE.md).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/api"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/config"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/gab"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/homelab"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/pipeline"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/state"
	"github.com/RyanRezaie/unified-dashboard-personal/web"
)

const version = "0.1.0"

func main() {
	log.SetFlags(log.LstdFlags)
	if err := run(); err != nil {
		log.Fatalf("dashboard: %v", err)
	}
}

func run() error {
	cfg := config.FromEnv()

	// ============================================================
	// STARTUP WARNINGS — say what is unconfigured rather than
	// failing silently or pretending a placeholder was a decision.
	// ============================================================
	if cfg.NoWatchedContainers() {
		log.Printf("dashboard: WARNING ATTENTION_CONTAINERS is empty, so " +
			"attention rule 2 (a watched container is down) can never fire.")
	}
	if cfg.Stub {
		log.Printf("dashboard: STUB MODE — serving fixture data, no real sources")
	} else {
		if cfg.ProxmoxURL == "" {
			log.Printf("dashboard: PROXMOX_URL unset — container/storage lane will show as failed")
		}
		if cfg.GPUAgentURL == "" {
			log.Printf("dashboard: GPU_AGENT_URL unset — GPU lane will show as failed")
		}
	}

	store, err := pipeline.Open(cfg.PipelinePath)
	if err != nil {
		return err
	}

	states := state.NewStore(
		cfg,
		gab.New(cfg.GABBaseURL, cfg.UpstreamTimeout),
		homelab.New(
			homelab.NewProxmox(cfg.ProxmoxURL, cfg.ProxmoxTokenID, cfg.ProxmoxSecret,
				cfg.ProxmoxNode, cfg.ProxmoxInsecure, cfg.UpstreamTimeout),
			homelab.NewGPUAgent(cfg.GPUAgentURL, cfg.UpstreamTimeout),
			cfg.AttentionContainers,
		),
		store,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Upstreams refresh on their own timer; UI polls only read memory.
	go states.Run(ctx)

	srv := &http.Server{
		Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler: api.New(cfg, states, store, web.FS()).Routes(),
		// An always-on panel holds a connection open indefinitely; only the
		// read side gets a deadline so a stuck client cannot pin a goroutine.
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("dashboard: shutdown: %v", err)
		}
	}()

	log.Printf("dashboard v%s — open %s (bound to %s)",
		version, browsableURL(cfg.Host, cfg.Port), srv.Addr)
	if cfg.Stub {
		// Printing "G.A.B. at ..." here was actively misleading: in stub mode
		// that address is never contacted, and the panel showing the mockup's
		// fixture data looks exactly like a real fetch that went wrong.
		fmt.Fprintf(os.Stderr, "dashboard: fixture data — G.A.B. at %s is NOT being read. "+
			"Drop DASHBOARD_STUB=1 for real data.\n", cfg.GABBaseURL)
	} else {
		fmt.Fprintf(os.Stderr, "dashboard: G.A.B. at %s · refresh every %s\n",
			cfg.GABBaseURL, cfg.RefreshInterval)
	}

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// browsableURL turns the bind address into one you can actually open.
//
// HOST is 0.0.0.0 on purpose (see config.HOST — reachability is Tailscale's
// job), but that is a wildcard meaning "every interface", not a destination.
// Pasted into a browser it goes nowhere; Chrome blocks 0.0.0.0 outright. The
// startup line is the one place that address gets read by a human, so it
// prints the loopback form and reports the real bind separately.
func browsableURL(host string, port int) string {
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}
