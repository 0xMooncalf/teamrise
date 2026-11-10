package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"ladder/internal/connections"
	"ladder/internal/discordbot"
	"ladder/internal/discordrooms"
	"ladder/internal/pipeline"
	"ladder/internal/providers"
	"ladder/internal/voice"
)

func New(engine *pipeline.Engine, assets fs.FS, stores ...*connections.Store) http.Handler {
	store, _ := connections.New("")
	if len(stores) > 0 {
		store = stores[0]
	}
	return NewWithDiscord(engine, assets, store, discordbot.New())
}

func NewWithDiscord(engine *pipeline.Engine, assets fs.FS, store *connections.Store, bot *discordbot.Manager, voices ...*voice.Service) http.Handler {
	var speech *voice.Service
	if len(voices) > 0 {
		speech = voices[0]
	}
	voiceStatus := func() voice.Status {
		if speech != nil {
			return speech.Status()
		}
		return voice.Status{State: "disabled", Language: "en"}
	}
	discordStatus := func() discordbot.Status {
		status := bot.Status()
		status.VoiceReady = voiceStatus().Ready && status.Connected
		return status
	}
	snapshotOf := func(state pipeline.State) map[string]any {
		b, _ := json.Marshal(state)
		var s map[string]any
		_ = json.Unmarshal(b, &s)
		s["connections"] = store.Public()
		s["discord_connection"] = discordStatus()
		s["voice_connection"] = voiceStatus()
		return s
	}
	snapshot := func() map[string]any { return snapshotOf(engine.Snapshot()) }
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("X-Frame-Options", "DENY")
			if !localHost(req.Host) {
				writeError(w, http.StatusForbidden, "use localhost or a loopback IP to access this local workspace")
				return
			}
			if req.Method == "POST" {
				binaryUpload := req.URL.Path == "/api/screenshots" && req.Header.Get("Content-Type") == "application/octet-stream"
				if !binaryUpload && !strings.HasPrefix(req.Header.Get("Content-Type"), "application/json") {
					writeError(w, http.StatusUnsupportedMediaType, "application/json required")
					return
				}
				if origin := req.Header.Get("Origin"); origin != "" {
					u, err := url.Parse(origin)
					if err != nil || u.Host != req.Host {
						writeError(w, http.StatusForbidden, "cross-origin request denied")
						return
					}
				}
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "name": "Teamrise", "mode": "demo-and-real-ai", "version": "0.9.3"})
	})
	r.Get("/api/voice/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, voiceStatus()) })
	r.Post("/api/voice/enable", func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !decode(w, r, &body) {
			return
		}
		if speech == nil {
			writeError(w, 503, "voice service is unavailable")
			return
		}
		if err := speech.Enable(r.Context()); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Get("/api/discord/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, discordStatus()) })
	r.Post("/api/discord/connect", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if !decode(w, r, &body) {
			return
		}
		if err := bot.Connect(r.Context(), body.Token); err != nil {
			writeError(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, bot.Status())
	})
	r.Post("/api/discord/disconnect", func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !decode(w, r, &body) {
			return
		}
		bot.Close()
		writeJSON(w, 200, bot.Status())
	})
	r.Post("/api/discord/reconnect", func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !decode(w, r, &body) {
			return
		}
		if err := bot.Reconnect(r.Context()); err != nil {
			writeError(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, bot.Status())
	})
	r.Post("/api/discord/details", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			GuildID    string `json:"guild_id"`
			CategoryID string `json:"category_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		details, err := bot.Details(r.Context(), body.GuildID, body.CategoryID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, details)
	})
	r.Post("/api/discord/configure", func(w http.ResponseWriter, r *http.Request) {
		var cfg connections.Discord
		if !decode(w, r, &cfg) {
			return
		}
		if err := bot.Verify(r.Context(), cfg); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		cfg.Enabled = true
		cfg.AutoCreateRooms = true
		if err := store.SaveDiscord(cfg); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Get("/api/connections", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, store.Public()) })
	r.Post("/api/connections/codex/status", func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !decode(w, r, &body) {
			return
		}
		writeJSON(w, 200, providers.LocalCodexStatus(r.Context()))
	})
	r.Post("/api/connections", func(w http.ResponseWriter, r *http.Request) {
		var u connections.Update
		if !decode(w, r, &u) {
			return
		}
		if u.Discord != nil && u.Discord.Enabled && bot.Status().HasToken {
			if err := bot.Verify(r.Context(), *u.Discord); err != nil {
				writeError(w, 400, err.Error())
				return
			}
		}
		if err := store.Save(u); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/connections/test", func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !decode(w, r, &body) {
			return
		}
		cfg, key := store.Credentials()
		if err := (providers.Compatible{Config: cfg, Key: key}).Test(r.Context()); err != nil {
			writeError(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	r.Post("/api/interviews", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Name            string `json:"name"`
			ProfileID       string `json:"profile_id"`
			BindingAgent    string `json:"binding_agent"`
			DiscordMemberID string `json:"discord_member_id"`
		}
		if !decode(w, r, &b) {
			return
		}
		var plan *discordrooms.Plan
		if b.DiscordMemberID != "" {
			var err error
			plan, err = discordrooms.Build(store.Public().Discord, b.DiscordMemberID)
			if err != nil {
				writeError(w, 400, err.Error())
				return
			}
		}
		id, err := engine.CreateInterview(b.Name, b.ProfileID, "", plan)
		if err != nil {
			handleError(w, err)
			return
		}
		s := snapshot()
		s["created_candidate_id"] = id
		writeJSON(w, 200, s)
	})
	r.Post("/api/candidates/{id}/ai/{action}", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Answer  string `json:"answer"`
			Version int    `json:"version"`
		}
		if !decode(w, r, &b) {
			return
		}
		cfg, key := store.Credentials()
		if c, err := engine.Candidate(chi.URLParam(r, "id")); err == nil && c.Live != nil && c.Live.Voice != nil {
			writeError(w, 409, "Voice interview turns are controlled in Discord. Use /interview-repeat or /interview-stop.")
			return
		}
		if err := engine.RunAI(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "action"), b.Answer, b.Version, providers.Compatible{Config: cfg, Key: key, Interview: store.Public().Interview}); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Get("/api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, snapshot()) })
	r.Get("/api/candidates/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, err := engine.Candidate(chi.URLParam(r, "id"))
		if err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, c)
	})
	r.Post("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
		var p pipeline.InterviewProfile
		if !decode(w, r, &p) {
			return
		}
		if err := engine.SaveProfile(p); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/interview-settings", func(w http.ResponseWriter, r *http.Request) {
		var cfg connections.Interview
		if !decode(w, r, &cfg) {
			return
		}
		if err := store.SaveInterview(cfg); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/screenshots", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, pipeline.MaxScreenshotBytes))
		if err != nil {
			writeError(w, 400, "could not read the screenshot (maximum 4 MB)")
			return
		}
		id, err := engine.SaveScreenshot(data)
		if err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 201, map[string]string{"id": id})
	})
	r.Get("/api/screenshots/{id}", func(w http.ResponseWriter, r *http.Request) {
		data, contentType, err := engine.Screenshot(chi.URLParam(r, "id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		_, _ = w.Write(data)
	})
	r.Post("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		var s pipeline.Settings
		if !decode(w, r, &s) {
			return
		}
		if err := engine.SaveSettings(s); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/candidates/{id}/profile", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ProfileID string `json:"profile_id"`
		}
		if !decode(w, r, &b) {
			return
		}
		if err := engine.AssignProfile(chi.URLParam(r, "id"), b.ProfileID); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/requests/{id}", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Action  string `json:"action"`
			AdminID string `json:"admin_id"`
		}
		if !decode(w, r, &b) {
			return
		}
		if err := engine.AdminRequest(chi.URLParam(r, "id"), b.Action, b.AdminID); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/candidates/{id}/room", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Action  string `json:"action"`
			AdminID string `json:"admin_id"`
		}
		if !decode(w, r, &b) {
			return
		}
		if err := engine.RoomAction(chi.URLParam(r, "id"), b.Action, b.AdminID); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/clock", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Seconds int `json:"seconds"`
		}
		if !decode(w, r, &b) {
			return
		}
		if err := engine.AdvanceTime(b.Seconds); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/demo/{action}", func(w http.ResponseWriter, r *http.Request) {
		if action := chi.URLParam(r, "action"); action != "start" && action != "pause" && action != "step" && action != "reset" {
			writeError(w, 400, "unknown demo action")
			return
		}
		var body struct{}
		if !decode(w, r, &body) {
			return
		}
		if err := engine.Control(chi.URLParam(r, "action")); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Post("/api/candidates/{id}/decision", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string `json:"action"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Action != "resolve" && body.Action != "hire" && body.Action != "reject" {
			writeError(w, 400, "unknown decision")
			return
		}
		if err := engine.Decision(chi.URLParam(r, "id"), body.Action); err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, 200, snapshot())
	})
	r.Get("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, 500, "streaming unavailable")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		last := ""
		for {
			state := snapshot()
			revision := fmt.Sprintf("%v-%v-%v-%v", state["revision"], state["connections"].(connections.Public).Revision, state["discord_connection"].(discordbot.Status).Revision, state["voice_connection"].(voice.Status).Revision)
			if revision != last {
				b, _ := json.Marshal(state)
				if _, err := fmt.Fprintf(w, "id: %s\ndata: %s\n\n", revision, b); err != nil {
					return
				}
				flusher.Flush()
				last = revision
			}
			select {
			case <-r.Context().Done():
				return
			case <-heartbeat.C:
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case <-ticker.C:
			}
		}
	})
	r.Get("/api/export", func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("workspace")
		if scope != "live" && scope != "demo" {
			w.Header().Set("Content-Disposition", "attachment; filename=teamrise-workspace.json")
			writeJSON(w, 200, snapshot())
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=teamrise-"+scope+".json")
		state := engine.Snapshot()
		live := scope == "live"
		state.Candidates = slices.DeleteFunc(state.Candidates, func(c pipeline.Candidate) bool { return (c.Mode == "live") != live })
		ids := make(map[string]bool, len(state.Candidates))
		if live {
			state.Demo, state.Running, state.ClockSeconds = false, false, 0
			state.Stats = pipeline.Stats{}
		}
		for _, c := range state.Candidates {
			ids[c.ID] = true
			if live {
				state.Stats.InterviewSeconds += c.InterviewSeconds
				if c.Assessment != nil {
					state.Stats.CompletedInterviews++
				}
				if c.Room.BotPresent {
					state.Stats.LiveRooms++
				}
				if c.Live != nil && (c.Live.Status == "generating" || c.Live.Status == "awaiting_answer" || c.Live.Status == "analyzing") {
					state.Stats.ActiveInterviews++
				}
			}
		}
		state.Requests = slices.DeleteFunc(state.Requests, func(r pipeline.AdminRequest) bool { return !ids[r.CandidateID] })
		state.Events = slices.DeleteFunc(state.Events, func(e pipeline.Event) bool {
			if !live && e.CandidateID == "" && (e.Kind == "progress" || e.Kind == "control") {
				return false
			}
			return !ids[e.CandidateID]
		})
		state.Stats.PendingRequests = 0
		for _, r := range state.Requests {
			if r.Status == "pending" {
				state.Stats.PendingRequests++
			}
		}
		writeJSON(w, 200, snapshotOf(state))
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, 404, "endpoint not found")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(assets, path); err != nil {
			if strings.Contains(path, ".") {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = "/"
		}
		http.FileServer(http.FS(assets)).ServeHTTP(w, r)
	})
	return r
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		writeError(w, 400, "invalid JSON body")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "expected one JSON object")
		return false
	}
	return true
}

func localHost(authority string) bool {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		host = strings.Trim(authority, "[]")
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pipeline.ErrValidation):
		writeError(w, 400, err.Error())
	case errors.Is(err, pipeline.ErrNotFound):
		writeError(w, 404, err.Error())
	case errors.Is(err, pipeline.ErrConflict):
		writeError(w, 409, err.Error())
	default:
		log.Printf("API persistence error: %v", err)
		writeError(w, 500, "could not save this action; check the server log")
	}
}
