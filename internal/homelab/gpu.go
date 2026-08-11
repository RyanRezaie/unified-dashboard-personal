package homelab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

// ============================================================
// GPU AGENT
//
// The dashboard does NOT run on the workstation (CLAUDE.md), so it cannot
// shell out to nvidia-smi. It reads a small Tailscale-reachable agent on the
// workstation instead.
//
// TOPOLOGY: the GPU (RTX 5070 Ti) and the LLM live on Ryan's main PC; this
// dashboard and everything else live on the always-on server. So this is the
// one upstream that crosses machines, and the one expected to be legitimately
// offline whenever the main PC is off. That is why an unreachable agent
// degrades only the GPU lane and never the panel.
//
// CONTRACT the agent must serve at GET /gpu — one field per nvidia-smi query
// column, so the agent stays a thin wrapper over:
//
//   nvidia-smi --query-gpu=name,temperature.gpu,fan.speed,memory.used,memory.total \
//              --format=csv,noheader,nounits
//
//   {"generated_at":"2026-08-11T21:47:03-05:00",
//    "gpus":[{"name":"NVIDIA GeForce RTX 3060","temp_c":61,"fan_pct":38,
//             "vram_used_mb":7400,"vram_total_mb":12288}]}
//
// The agent itself is not in this repo — it's a separate service on the
// workstation, and per Ryan's note it will most likely be Go too.
// ============================================================

const gpuAgentPath = "/gpu"

type GPUAgent struct {
	baseURL string
	http    *http.Client
}

func NewGPUAgent(baseURL string, timeout time.Duration) *GPUAgent {
	return &GPUAgent{baseURL: baseURL, http: &http.Client{Timeout: timeout}}
}

func (g *GPUAgent) Fetch(ctx context.Context) ([]model.GPU, error) {
	if g.baseURL == "" {
		return nil, fmt.Errorf("gpu: no GPU_AGENT_URL configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.baseURL+gpuAgentPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gpu: agent unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gpu: agent returned %s", resp.Status)
	}

	var body struct {
		GPUs []model.GPU `json:"gpus"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("gpu: bad json: %w", err)
	}
	return body.GPUs, nil
}
