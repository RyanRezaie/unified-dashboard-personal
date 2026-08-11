package homelab

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

// ============================================================
// PROXMOX — container/VM state and storage, via API token auth.
//
// One call to /api2/json/cluster/resources gets both guests and storage,
// which is cheaper than walking nodes and is correct on a single-node setup
// as well as a cluster.
// ============================================================

const proxmoxResourcesPath = "/api2/json/cluster/resources"

type Proxmox struct {
	baseURL string
	tokenID string
	secret  string
	node    string // "" = all nodes
	http    *http.Client
}

func NewProxmox(baseURL, tokenID, secret, node string, insecure bool, timeout time.Duration) *Proxmox {
	// InsecureSkipVerify is the default for homelab Proxmox, which ships a
	// self-signed cert. It is a config flag rather than hardcoded so a proper
	// cert can be used later by setting PROXMOX_INSECURE=false.
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	return &Proxmox{
		baseURL: baseURL,
		tokenID: tokenID,
		secret:  secret,
		node:    node,
		http:    &http.Client{Timeout: timeout, Transport: transport},
	}
}

// resource is the subset of a cluster/resources row this dashboard reads.
type resource struct {
	Type    string  `json:"type"` // "lxc" | "qemu" | "storage" | "node"
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Node    string  `json:"node"`
	VMID    int     `json:"vmid"`
	Storage string  `json:"storage"`
	Disk    float64 `json:"disk"`
	MaxDisk float64 `json:"maxdisk"`
}

// Fetch returns containers/VMs and storage pools in one round trip.
func (p *Proxmox) Fetch(ctx context.Context, watched []string) ([]model.Container, []model.Disk, error) {
	if p.baseURL == "" {
		return nil, nil, fmt.Errorf("proxmox: no PROXMOX_URL configured")
	}
	if p.tokenID == "" || p.secret == "" {
		return nil, nil, fmt.Errorf("proxmox: PROXMOX_TOKEN_ID / PROXMOX_TOKEN_SECRET not set")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+proxmoxResourcesPath, nil)
	if err != nil {
		return nil, nil, err
	}
	// Proxmox API-token form. Note this is not a Bearer token.
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", p.tokenID, p.secret))

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("proxmox: unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("proxmox: returned %s", resp.Status)
	}

	var body struct {
		Data []resource `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, nil, fmt.Errorf("proxmox: bad json: %w", err)
	}

	isWatched := make(map[string]bool, len(watched))
	for _, name := range watched {
		isWatched[name] = true
	}

	var containers []model.Container
	var disks []model.Disk
	for _, r := range body.Data {
		if p.node != "" && r.Node != p.node && r.Type != "storage" {
			continue
		}
		switch r.Type {
		case "lxc", "qemu":
			containers = append(containers, model.Container{
				Name:    r.Name,
				Status:  r.Status,
				Running: r.Status == "running",
				Node:    r.Node,
				VMID:    r.VMID,
				Watched: isWatched[r.Name],
			})
		case "storage":
			if r.MaxDisk <= 0 {
				continue // inactive/unsized pool; a 0-byte bar is noise
			}
			disks = append(disks, model.Disk{
				Name:       r.Storage,
				UsedPct:    r.Disk / r.MaxDisk * 100,
				UsedBytes:  int64(r.Disk),
				TotalBytes: int64(r.MaxDisk),
			})
		}
	}
	return containers, disks, nil
}
