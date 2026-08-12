// Package api wires the HTTP surface.
//
// Two rules shape this file:
//   - Everything the monitor needs is one GET /api/state. The UI polls once
//     every 20s; it should not have to fan out.
//   - The only writes in the whole dashboard are the pipeline routes. There is
//     no route here that writes to G.A.B., by construction.
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/RyanRezaie/unified-dashboard-personal/internal/config"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/model"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/pipeline"
	"github.com/RyanRezaie/unified-dashboard-personal/internal/state"
)

// UIPollSeconds is the client's data poll interval, pinned at 20s in
// docs/dashboard-ui.md. Served to the frontend so the number lives in one place.
const UIPollSeconds = 20

type Server struct {
	cfg      config.Config
	state    *state.Store
	pipeline *pipeline.Store
	web      fs.FS
}

func New(cfg config.Config, st *state.Store, p *pipeline.Store, web fs.FS) *Server {
	return &Server{cfg: cfg, state: st, pipeline: p, web: web}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// --- UI ---
	mux.Handle("GET /", staticHandler(s.web))

	// --- read ---
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/pipeline", s.handlePipelineList)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// --- write (pipeline only) ---
	mux.HandleFunc("POST /api/pipeline", s.handlePipelineAdd)
	mux.HandleFunc("PATCH /api/pipeline/{id}", s.handlePipelineUpdate)
	mux.HandleFunc("DELETE /api/pipeline/{id}", s.handlePipelineDelete)

	return logging(mux)
}

// ============================================================
// STATIC ASSETS
// ============================================================

// staticHandler serves the embedded frontend with a content-hash ETag and
// Cache-Control: no-cache.
//
// Without it there is no validator on index.html at all. embed.FS reports a
// ZERO modtime, so http.FileServerFS sends neither Last-Modified nor ETag,
// and a browser already holding the file has no way to ask whether it
// changed. That turns a shipped frontend fix into one that appears not to
// work — the phone keeps running the old JS after a correct rebuild and
// redeploy, which is indistinguishable from the bug never having been fixed.
//
// Hashing once at startup is exact rather than approximate: the assets are
// compiled into the binary, so they cannot change while the process runs,
// and the tag changes if and only if a rebuild changed the bytes.
//
// "no-cache" means "keep it, but revalidate before every use" — not "do not
// store". Paired with the ETag, an unchanged panel costs one 304 and no body.
func staticHandler(files fs.FS) http.Handler {
	etags := map[string]string{}
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(files, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		// Quoted because an ETag is a quoted-string on the wire; net/http
		// compares it verbatim against If-None-Match. Half the digest is
		// plenty to tell two builds apart.
		etags["/"+p] = `"` + hex.EncodeToString(sum[:8]) + `"`
		return nil
	})
	if err != nil {
		// Only reachable if the embedded FS itself is unreadable, which is a
		// build-time mistake rather than a runtime condition.
		panic(err)
	}

	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// FileServerFS resolves a directory to its index.html; the hash map
		// is keyed by real file, so resolve the same way before looking up.
		p := r.URL.Path
		if strings.HasSuffix(p, "/") {
			p += "index.html"
		}
		if tag, ok := etags[p]; ok {
			// Set BEFORE delegating: net/http's conditional-request check
			// reads the ETag back off the response header, so this is what
			// turns a matching If-None-Match into a 304.
			w.Header().Set("Etag", tag)
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// ============================================================
// READ HANDLERS
// ============================================================

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.state.Snapshot())
}

// handleConfig serves the values the frontend needs but must not hardcode:
// the poll interval and the burn-in mitigation settings, which
// docs/dashboard-ui.md wants declared as constants rather than scattered.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		PollSeconds int                `json:"poll_seconds"`
		Drift       config.DriftConfig `json:"burn_in"`
		Stub        bool               `json:"stub"`
		Stages      []model.Stage      `json:"stages"`
		// Thresholds are sent so the UI can paint a meter amber at the same
		// point the attention rule fires. The rule itself stays server-side —
		// this is only which bar turns colour.
		Thresholds struct {
			GPUTempC float64 `json:"gpu_temp_c"`
			DiskPct  float64 `json:"disk_pct"`
		} `json:"thresholds"`
	}{
		PollSeconds: UIPollSeconds,
		Drift:       s.cfg.Drift,
		Stub:        s.cfg.Stub,
		Stages:      model.Stages,
		Thresholds: struct {
			GPUTempC float64 `json:"gpu_temp_c"`
			DiskPct  float64 `json:"disk_pct"`
		}{s.cfg.AttentionGPUTempC, s.cfg.AttentionDiskPct},
	})
}

func (s *Server) handlePipelineList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.pipeline.All()})
}

// ============================================================
// WRITE HANDLERS — pipeline only
// ============================================================

func (s *Server) handlePipelineAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Company string      `json:"company"`
		Role    string      `json:"role"`
		Stage   model.Stage `json:"stage"`
		Note    string      `json:"note"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Stage == "" {
		body.Stage = model.StageEmailed // the first stage in the funnel
	}

	item, err := s.pipeline.Add(body.Company, body.Role, body.Stage, body.Note)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handlePipelineUpdate(w http.ResponseWriter, r *http.Request) {
	// Pointers so that "field absent" and "field set to empty" stay distinct —
	// the phone's tap-to-advance sends only {"stage": "..."}.
	var body struct {
		Company *string      `json:"company"`
		Role    *string      `json:"role"`
		Note    *string      `json:"note"`
		Stage   *model.Stage `json:"stage"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	item, err := s.pipeline.Update(r.PathValue("id"), body.Company, body.Role, body.Note, body.Stage)
	switch {
	case errors.Is(err, pipeline.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case err != nil:
		writeError(w, http.StatusBadRequest, err)
	default:
		writeJSON(w, http.StatusOK, item)
	}
}

func (s *Server) handlePipelineDelete(w http.ResponseWriter, r *http.Request) {
	err := s.pipeline.Delete(r.PathValue("id"))
	switch {
	case errors.Is(err, pipeline.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// ============================================================
// HELPERS
// ============================================================

// maxBody caps request bodies. This is a single-user LAN tool, but an
// unbounded decode is never worth leaving open.
const maxBody = 64 << 10

func decode(r *http.Request, into any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return errors.New("invalid request body: " + err.Error())
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	// The panel polls every 20s; a cached response would defeat the point.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("api: write failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// logging logs writes and failures only. Logging every 20s poll would bury
// anything useful within a day of the panel being on.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("api: %s %s → %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
