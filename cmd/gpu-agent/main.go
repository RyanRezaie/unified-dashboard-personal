// Command gpu-agent exposes this machine's NVIDIA GPUs over HTTP.
//
// It runs on the MAIN PC (the RTX 5070 Ti box), not on the always-on server:
// the dashboard deliberately does not run where the GPU is (CLAUDE.md), so it
// cannot shell out to nvidia-smi and reads this instead, over Tailscale.
//
// It is intentionally the thinnest possible wrapper. One exec of nvidia-smi
// per request, parsed into the shape internal/homelab/gpu.go already pins,
// and no state of any kind. Everything hard about the GPU lane — caching,
// degradation, thresholds — is already solved on the dashboard side, and
// duplicating any of it here would mean two places to fix it.
//
// NOT A CONTAINER, on purpose. Reaching a GPU from inside one needs the
// NVIDIA container toolkit installed and a device reservation that fails
// closed when the card is absent — real setup cost for a single static
// binary whose whole job is to run one command. `deploy/gpu-agent.service`
// is a systemd user unit that does the same thing with nothing to install.
//
//	go build -o gpu-agent ./cmd/gpu-agent && ./gpu-agent
//	curl -s localhost:8883/gpu | python3 -m json.tool
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
)

// ============================================================
// CONFIG — top of file, env-overridable, no secrets anywhere
// ============================================================

const (
	defaultHost = "0.0.0.0" // every interface: the point is to be reachable
	// from the always-on server over Tailscale. A wildcard bind, not an
	// address a browser can visit — same note as the dashboard's.
	defaultPort = "8883" // by agreement with the rest of the homelab:
	// 8080 SearXNG, 8880 ntfy, 8881 dashboard, 8882 G.A.B., 8883 here.

	defaultSmiBin = "nvidia-smi"

	// The exact query internal/homelab/gpu.go documents. One column per
	// field of model.GPU, in this order — the parser below depends on the
	// count and the order, and nothing else does.
	smiQuery  = "name,temperature.gpu,fan.speed,memory.used,memory.total"
	smiFormat = "csv,noheader,nounits"

	// A healthy nvidia-smi answers in well under a second. This bound
	// exists for the unhealthy case: a hung driver makes it block
	// indefinitely, and without a deadline that becomes a stuck request
	// holding the dashboard's own 20s poll open.
	smiTimeout = 5 * time.Second

	// Generous because the only client polls every 20 seconds and is on
	// the far side of a Tailscale hop.
	readTimeout  = 10 * time.Second
	writeTimeout = 15 * time.Second
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ============================================================
// NVIDIA-SMI
// ============================================================

// queryGPUs runs nvidia-smi once and parses every line it prints.
func queryGPUs(ctx context.Context, bin string) ([]model.GPU, error) {
	ctx, cancel := context.WithTimeout(ctx, smiTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin,
		"--query-gpu="+smiQuery,
		"--format="+smiFormat)
	out, err := cmd.Output()
	if err != nil {
		// Three failures worth telling apart, because the fixes are
		// unrelated: the tool is missing, the driver is not loaded (smi
		// exits non-zero and explains why on stderr), or it hung.
		//
		// "Missing" arrives as two different errors: a bare name that PATH
		// lookup missed gives ErrNotFound, while an absolute NVIDIA_SMI_BIN
		// pointing nowhere gives a PathError. Both mean the same thing to
		// whoever is reading the log.
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s not found — is the NVIDIA driver installed on this machine?", bin)
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s timed out after %s — driver may be wedged", bin, smiTimeout)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%s failed: %s", bin, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("%s failed: %w", bin, err)
	}
	return parseSMI(string(out))
}

// parseSMI turns nvidia-smi's CSV into GPUs. One line per card.
//
// Split from the RIGHT rather than the left: the four numeric columns are
// fixed, and the name is whatever precedes them. A card whose model name
// contains a comma would otherwise shift every field by one and report a
// plausible-looking wrong temperature, which is worse than an error.
func parseSMI(out string) ([]model.GPU, error) {
	var gpus []model.GPU
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 5 {
			return nil, fmt.Errorf("unexpected nvidia-smi output %q — expected %s", line, smiQuery)
		}
		nums := fields[len(fields)-4:]
		name := strings.TrimSpace(strings.Join(fields[:len(fields)-4], ","))

		gpus = append(gpus, model.GPU{
			Name:        name,
			TempC:       parseNum(nums[0]),
			FanPct:      parseNum(nums[1]),
			VRAMUsedMB:  parseNum(nums[2]),
			VRAMTotalMB: parseNum(nums[3]),
		})
	}
	if len(gpus) == 0 {
		return nil, errors.New("nvidia-smi listed no GPUs")
	}
	return gpus, nil
}

// parseNum reads one column, treating anything unparseable as 0.
//
// "[N/A]" is the case this exists for, and it is not an error: plenty of
// cards never report fan speed (blower-less, laptop, and datacenter cards
// all do this), and a 500 because one optional column is absent would take
// down a lane that is otherwise working perfectly. 0 shows as 0% on the
// panel, which is the truth available.
func parseNum(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// ============================================================
// HTTP
// ============================================================

type response struct {
	GeneratedAt string      `json:"generated_at"`
	GPUs        []model.GPU `json:"gpus"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("gpu-agent: write failed: %v", err)
	}
}

func handleGPU(bin string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET only"})
			return
		}
		gpus, err := queryGPUs(r.Context(), bin)
		if err != nil {
			// 503, not 500: the dashboard reads any non-200 as a failed
			// GPU lane and carries on, which is exactly right here — the
			// card being unavailable is a fact about the machine, not a
			// bug in this program.
			log.Printf("gpu-agent: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{
			GeneratedAt: time.Now().Format(time.RFC3339),
			GPUs:        gpus,
		})
	}
}

// handleHealthz answers 200 whenever the process is up, deliberately WITHOUT
// touching nvidia-smi. Liveness is "this program is running", and tying it to
// the driver would have systemd restarting a perfectly good agent every time
// the card hiccuped — which fixes nothing and hides the real symptom.
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func main() {
	log.SetFlags(log.LstdFlags)
	if err := run(); err != nil {
		log.Fatalf("gpu-agent: %v", err)
	}
}

func run() error {
	var (
		host = env("GPU_AGENT_HOST", defaultHost)
		port = env("GPU_AGENT_PORT", defaultPort)
		bin  = env("NVIDIA_SMI_BIN", defaultSmiBin)
	)
	addr := net.JoinHostPort(host, port)

	// Said at startup rather than only on the first request. An agent that
	// comes up clean and fails every poll from another machine is a
	// genuinely annoying thing to debug from the dashboard's side.
	if gpus, err := queryGPUs(context.Background(), bin); err != nil {
		log.Printf("gpu-agent: WARNING %v", err)
		log.Printf("gpu-agent: starting anyway — /gpu will return 503 until this is fixed, " +
			"and the dashboard shows the GPU lane as failed rather than going down with it")
	} else {
		for _, g := range gpus {
			log.Printf("gpu-agent: found %s (%.0f°C, %.0f/%.0f MB VRAM)",
				g.Name, g.TempC, g.VRAMUsedMB, g.VRAMTotalMB)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/gpu", handleGPU(bin))
	mux.HandleFunc("/healthz", handleHealthz)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	// Ctrl-C and `systemctl stop` both land here.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	errc := make(chan error, 1)
	go func() {
		log.Printf("gpu-agent: listening on %s (GET /gpu)", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-stop:
		log.Printf("gpu-agent: shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
