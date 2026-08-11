// Package config holds every tunable in one place.
//
// Repo convention (see CLAUDE.md): config constants live at the top of the
// file, not scattered through the code, and no API keys are hardcoded —
// secrets come from the environment only.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// ============================================================
// CONSTANTS — defaults. Every one is overridable by env var
// (see FromEnv) so the container needs no rebuild to retune.
// ============================================================

const (
	// --- server ---
	// HOST binds all interfaces because reachability is handled one layer up
	// by Tailscale ACLs, per the "no auth beyond Tailscale" non-goal.
	HOST = "0.0.0.0"
	PORT = 8088

	// --- upstreams ---
	// G.A.B.'s read-only assignments endpoint. Local-only; never written to.
	GABBaseURL = "http://127.0.0.1:8080"
	// The workstation agent exposing nvidia-smi. The dashboard does NOT run
	// on the workstation, so this is always a remote (Tailscale) address.
	GPUAgentURL     = ""
	ProxmoxURL      = ""
	ProxmoxNode     = ""
	ProxmoxInsecure = true // homelab Proxmox uses a self-signed cert by default

	// --- local state ---
	PipelinePath = "./data/pipeline.json"

	// --- polling ---
	// The UI polls every 20s (docs/dashboard-ui.md). The server refreshes its
	// upstream snapshot on its own timer so a slow Proxmox call never makes a
	// UI poll hang; RefreshInterval is deliberately shorter than the UI's 20s
	// so a poll almost always finds fresh data.
	RefreshInterval = 10 * time.Second
	UpstreamTimeout = 5 * time.Second

	// --- burn-in mitigation (docs/dashboard-ui.md) ---
	// The panel shows this UI ~99% of the time. Both values are read by the
	// frontend via /api/config so they stay declared in one place.
	DriftPx        = 6  // peak layout offset, pixels
	DriftPeriodMin = 17 // minutes for a full drift cycle
	DimStartHour   = 23 // dim from this hour (local, 24h)
	DimEndHour     = 7  // ...until this hour
	DimOpacity     = 0.55
)

// ============================================================
// ATTENTION THRESHOLDS
//
// !! UNCONFIRMED — PLACEHOLDERS, NOT DECISIONS !!
//
// docs/dashboard-ui.md: "Do not ship the placeholders as if they were
// chosen. Ask." These are carried over verbatim from that document and are
// still waiting on real values from Ryan:
//   - which containers are actually load-bearing
//   - what GPU temp is too hot for this card
//   - what disk percentage actually matters on the NAS
// Until then the dashboard logs a warning at startup naming this block.
// ============================================================

var (
	AttentionContainers = []string{} // UNCONFIRMED — empty: rule 2 never fires
	AttentionGPUTempC   = 80.0       // UNCONFIRMED
	AttentionDiskPct    = 90.0       // UNCONFIRMED
)

// ThresholdsUnconfirmed reports whether the attention rule is still running on
// placeholders, so main can say so out loud rather than pretending.
func (c Config) ThresholdsUnconfirmed() bool {
	return len(c.AttentionContainers) == 0
}

// ============================================================
// CONFIG
// ============================================================

type Config struct {
	Host string
	Port int

	GABBaseURL      string
	GPUAgentURL     string
	ProxmoxURL      string
	ProxmoxNode     string
	ProxmoxTokenID  string // e.g. "dashboard@pve!readonly" — env only
	ProxmoxSecret   string // env only
	ProxmoxInsecure bool

	PipelinePath string

	RefreshInterval time.Duration
	UpstreamTimeout time.Duration

	AttentionContainers []string
	AttentionGPUTempC   float64
	AttentionDiskPct    float64

	Drift DriftConfig

	// Stub serves fixture data for every remote source. This is how the UI
	// is developed without a homelab attached; it is never a fallback — if a
	// real source fails, the UI shows it as failed rather than showing fiction.
	Stub bool
}

type DriftConfig struct {
	Px           int     `json:"drift_px"`
	PeriodMin    int     `json:"drift_period_min"`
	DimStartHour int     `json:"dim_start_hour"`
	DimEndHour   int     `json:"dim_end_hour"`
	DimOpacity   float64 `json:"dim_opacity"`
}

// FromEnv builds a Config from the constants above, letting env vars override.
// Secrets (the Proxmox token) have no constant default on purpose.
func FromEnv() Config {
	return Config{
		Host: env("DASHBOARD_HOST", HOST),
		Port: envInt("DASHBOARD_PORT", PORT),

		GABBaseURL:      strings.TrimRight(env("GAB_URL", GABBaseURL), "/"),
		GPUAgentURL:     strings.TrimRight(env("GPU_AGENT_URL", GPUAgentURL), "/"),
		ProxmoxURL:      strings.TrimRight(env("PROXMOX_URL", ProxmoxURL), "/"),
		ProxmoxNode:     env("PROXMOX_NODE", ProxmoxNode),
		ProxmoxTokenID:  env("PROXMOX_TOKEN_ID", ""),
		ProxmoxSecret:   env("PROXMOX_TOKEN_SECRET", ""),
		ProxmoxInsecure: envBool("PROXMOX_INSECURE", ProxmoxInsecure),

		PipelinePath: env("PIPELINE_PATH", PipelinePath),

		RefreshInterval: envDur("REFRESH_INTERVAL", RefreshInterval),
		UpstreamTimeout: envDur("UPSTREAM_TIMEOUT", UpstreamTimeout),

		AttentionContainers: envList("ATTENTION_CONTAINERS", AttentionContainers),
		AttentionGPUTempC:   envFloat("ATTENTION_GPU_TEMP_C", AttentionGPUTempC),
		AttentionDiskPct:    envFloat("ATTENTION_DISK_PCT", AttentionDiskPct),

		Drift: DriftConfig{
			Px:           envInt("DRIFT_PX", DriftPx),
			PeriodMin:    envInt("DRIFT_PERIOD_MIN", DriftPeriodMin),
			DimStartHour: envInt("DIM_START_HOUR", DimStartHour),
			DimEndHour:   envInt("DIM_END_HOUR", DimEndHour),
			DimOpacity:   envFloat("DIM_OPACITY", DimOpacity),
		},

		Stub: envBool("DASHBOARD_STUB", false),
	}
}

// ============================================================
// ENV HELPERS — a bad value falls back to the default rather than
// failing to boot: an always-on panel should come up degraded, not not at all.
// ============================================================

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

// envList parses a comma-separated list, trimming blanks.
func envList(key string, def []string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
