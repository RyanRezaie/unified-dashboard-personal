// Package homelab aggregates the two homelab sources — Proxmox for
// container/VM and storage state, and a workstation agent for GPU metrics.
//
// The two are fetched concurrently and reported independently: a dead GPU
// agent must not blank the container list, and vice versa. That is what the
// separate SourceStatus fields on Snapshot are for.
package homelab

import (
	"context"
	"sync"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

type Snapshot struct {
	GPUs       []model.GPU       `json:"gpus"`
	Containers []model.Container `json:"containers"`
	Disks      []model.Disk      `json:"disks"`

	ProxmoxStatus model.SourceStatus `json:"proxmox_status"`
	GPUStatus     model.SourceStatus `json:"gpu_status"`
}

// Down returns the watched containers that are not running. This is attention
// rule 2, and it is deliberately restricted to the watched list — an unwatched
// container being stopped is normal, not a fault.
func (s Snapshot) Down() []model.Container {
	var down []model.Container
	for _, c := range s.Containers {
		if c.Watched && !c.Running {
			down = append(down, c)
		}
	}
	return down
}

// RunningWatched counts watched containers that are up, for the "4 / 5" tails.
func (s Snapshot) RunningWatched() (up, total int) {
	for _, c := range s.Containers {
		if c.Watched {
			total++
			if c.Running {
				up++
			}
		}
	}
	return up, total
}

type Client struct {
	proxmox *Proxmox
	gpu     *GPUAgent
	watched []string
}

func New(proxmox *Proxmox, gpu *GPUAgent, watched []string) *Client {
	return &Client{proxmox: proxmox, gpu: gpu, watched: watched}
}

// Fetch runs both sources in parallel. Neither can fail the other.
func (c *Client) Fetch(ctx context.Context) Snapshot {
	var (
		snap Snapshot
		wg   sync.WaitGroup
	)
	now := time.Now()
	snap.ProxmoxStatus.FetchedAt = now
	snap.GPUStatus.FetchedAt = now

	wg.Add(2)
	go func() {
		defer wg.Done()
		containers, disks, err := c.proxmox.Fetch(ctx, c.watched)
		if err != nil {
			snap.ProxmoxStatus.Error = err.Error()
			return
		}
		snap.Containers, snap.Disks = containers, disks
		snap.ProxmoxStatus.OK = true
	}()
	go func() {
		defer wg.Done()
		gpus, err := c.gpu.Fetch(ctx)
		if err != nil {
			snap.GPUStatus.Error = err.Error()
			return
		}
		snap.GPUs = gpus
		snap.GPUStatus.OK = true
	}()
	wg.Wait()

	return snap
}
