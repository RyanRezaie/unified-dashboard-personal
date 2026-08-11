package homelab

import (
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

// ============================================================
// STUB — see internal/gab/stub.go for the rule: fixtures are for
// DASHBOARD_STUB development only, never a fallback for a live failure.
//
// qdrant is down here on purpose so the NEEDS YOU panel, the amber core
// ring and the rail badge are all exercised, matching the mockup.
// ============================================================

func StubSnapshot(now time.Time, watched []string) Snapshot {
	isWatched := func(name string) bool {
		for _, w := range watched {
			if w == name {
				return true
			}
		}
		// With the watched list still UNCONFIRMED (and therefore empty), treat
		// every stub container as watched so the rule is visibly exercised.
		return len(watched) == 0
	}

	container := func(name, status string, vmid int) model.Container {
		return model.Container{
			Name: name, Status: status, Running: status == "running",
			Node: "pve", VMID: vmid, Watched: isWatched(name),
		}
	}

	ok := model.SourceStatus{OK: true, FetchedAt: now}
	return Snapshot{
		GPUs: []model.GPU{{
			Name: "NVIDIA GeForce RTX 3060", TempC: 61, FanPct: 38,
			VRAMUsedMB: 7400, VRAMTotalMB: 12288,
		}},
		Containers: []model.Container{
			container("ollama", "running", 101),
			container("searxng", "running", 102),
			container("ntfy", "running", 103),
			container("open-webui", "running", 104),
			container("qdrant", "stopped", 105),
		},
		Disks: []model.Disk{
			{Name: "nas", UsedPct: 44, UsedBytes: 8_800_000_000_000, TotalBytes: 20_000_000_000_000},
		},
		ProxmoxStatus: ok,
		GPUStatus:     ok,
	}
}
