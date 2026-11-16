// Package httpapi is the JSON/SSE HTTP layer of the API service.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/httpx"
)

const maxBodyBytes = 32 << 10

// Info is what the UI needs to know about the running configuration.
type Info struct {
	LLMMode string
	Model   string // only reported for the real model
	// WorkerHealthURL is the worker's /healthz. The browser cannot reach the
	// worker, so /api/system asks it from here. Empty = not configured.
	WorkerHealthURL string
}

// New builds the router. ready is the database readiness check.
func New(svc *campaign.Service, info Info, ready func(context.Context) error, metricsHandler http.Handler, log *slog.Logger) http.Handler {
	a := &api{svc: svc, info: info, ready: ready, log: log}
	mux := http.NewServeMux()
	// /healthz is what docker-compose asks: it includes the database. A process
	// that cannot reach its database is not healthy.
	mux.HandleFunc("GET /healthz", httpx.HealthzWith(ready))
	mux.HandleFunc("GET /readyz", httpx.Readyz(ready))
	mux.HandleFunc("GET /api/info", a.getInfo)
	mux.HandleFunc("GET /api/system", a.getSystem)
	mux.HandleFunc("POST /api/campaigns", a.createCampaign)
	mux.HandleFunc("GET /api/campaigns", a.listCampaigns)
	mux.HandleFunc("GET /api/campaigns/{id}", a.getCampaign)
	mux.HandleFunc("POST /api/campaigns/{id}/messages/stream", a.streamMessages)
	mux.HandleFunc("PUT /api/campaigns/{id}/content", a.putContent)
	mux.HandleFunc("POST /api/campaigns/{id}/approve", a.approve)
	mux.HandleFunc("POST /api/campaigns/{id}/cancel", a.cancel)
	if metricsHandler != nil {
		mux.Handle("GET /metrics", metricsHandler)
	}
	return mux
}

type api struct {
	svc   *campaign.Service
	info  Info
	ready func(context.Context) error
	log   *slog.Logger
}

// getSystem answers "is everything up?" for the dashboard: this api (it is
// answering), its database, and the worker, which is asked over the network.
// Each value is "ok", "down" or, when no worker address is configured, "unknown".
func (a *api) getSystem(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	db, worker := "ok", "unknown"
	if err := a.ready(ctx); err != nil {
		db = "down"
	}
	if a.info.WorkerHealthURL != "" {
		worker = "down"
		if req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.info.WorkerHealthURL, nil); err == nil {
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					worker = "ok"
				}
			}
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"api": "ok", "database": db, "worker": worker})
}

func (a *api) getInfo(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{
		"llm_mode": a.info.LLMMode,
		"limits": map[string]int{
			"sms_max":        content.SMSMaxRunes,
			"subject_max":    content.SubjectMaxRunes,
			"email_body_max": content.EmailBodyMaxRunes,
		},
		"variables":        a.svc.Policy().Infos(),
		"languages":        content.Languages,
		"default_language": content.DefaultLanguage,
	}
	if a.info.LLMMode == "claude" {
		out["model"] = a.info.Model
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *api) createCampaign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Brief string `json:"brief"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, err := a.svc.Create(r.Context(), body.Brief)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, v)
}

func (a *api) getCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := a.svc.Get(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (a *api) putContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Either one text (the default language's: {"sms_body": ...}) or several
	// ({"languages": {"tr": {...}, "en": {...}}}); the whole set must agree.
	var body struct {
		content.Content
		Languages map[string]content.Content `json:"languages"`
	}
	if !decode(w, r, &body) {
		return
	}
	var v *campaign.View
	var err error
	if body.Languages != nil {
		if body.Content != (content.Content{}) {
			writeError(w, http.StatusBadRequest, "invalid_input", "ya tek metin ya da languages gönderin, ikisini birden değil", nil)
			return
		}
		v, err = a.svc.SaveContents(r.Context(), id, body.Languages)
	} else {
		v, err = a.svc.SaveContent(r.Context(), id, body.Content)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (a *api) approve(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// No body sends now; {"scheduled_at": "<RFC 3339>"} schedules the sending.
	var body struct {
		ScheduledAt *time.Time `json:"scheduled_at"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	res, err := a.svc.ApproveAt(r.Context(), id, body.ScheduledAt)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// cancel cancels a scheduled campaign that has not started yet.
func (a *api) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := a.svc.CancelScheduled(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (a *api) streamMessages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s := newSSE(w, r.Context())
	defer s.close()

	err := a.svc.GenerateMessages(r.Context(), id, campaign.Events{
		Attempt: func(lang string, n, max int) {
			s.send("attempt", map[string]any{"language": lang, "n": n, "max": max})
		},
		Delta: func(lang, field, text string) {
			s.send("delta", map[string]string{"language": lang, "field": field, "text": text})
		},
		Retry: func(lang string, next int, problems []string) {
			s.send("retry", map[string]any{"language": lang, "next": next, "problems": problems})
		},
		Validated: func(lang string, c content.Content, rep content.Report) {
			s.send("validated", map[string]any{"language": lang, "content": c, "report": rep, "source": campaign.SourceLLM})
		},
		Fallback: func(lang, reason string, c content.Content, rep content.Report) {
			s.send("fallback", map[string]any{"language": lang, "reason": reason, "content": c, "report": rep, "source": campaign.SourceFallback})
		},
	})
	if err != nil {
		if s.started {
			// Headers are already out; report the failure as an event.
			if r.Context().Err() == nil {
				code, _, msg, _ := classify(err)
				s.send("error", map[string]string{"code": code, "message": msg})
			}
			return
		}
		a.fail(w, r, err)
		return
	}
	s.send("done", map[string]string{"status": campaign.StatusContentReady})
}

// ---- helpers --------------------------------------------------------------

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "not_found", "kampanya bulunamadı", nil)
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "istek gövdesi geçerli bir JSON değil: "+err.Error(), nil)
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "istek gövdesinde fazladan içerik var", nil)
		return false
	}
	return true
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	var b errorBody
	b.Error.Code, b.Error.Message, b.Error.Details = code, message, details
	httpx.WriteJSON(w, status, b)
}

// classify maps a service error to (code, status, message, details).
func classify(err error) (string, int, string, any) {
	var (
		input   *campaign.InputError
		state   *campaign.StateError
		invalid *campaign.InvalidContentError
		gen     *campaign.GenerationError
	)
	switch {
	case errors.Is(err, campaign.ErrNotFound):
		return "not_found", http.StatusNotFound, "kampanya bulunamadı", nil
	case errors.As(err, &input):
		return "invalid_input", http.StatusBadRequest, input.Message, nil
	case errors.As(err, &state):
		return "invalid_state", http.StatusConflict, state.Message, nil
	case errors.As(err, &invalid):
		return "invalid_content", http.StatusUnprocessableEntity, "içerik karakter sınırlarına veya kurallara uymuyor", map[string]any{"report": invalid.Report}
	case errors.As(err, &gen):
		status := http.StatusUnprocessableEntity
		switch gen.Code {
		case campaign.CodeLLMUnavailable:
			status = http.StatusBadGateway
		case campaign.CodeLLMMisconfigured:
			status = http.StatusServiceUnavailable
		}
		var details any
		if len(gen.Attempts) > 0 {
			details = map[string]any{"attempts": gen.Attempts}
		}
		return gen.Code, status, gen.Message, details
	}
	return "internal", http.StatusInternalServerError, "beklenmeyen bir hata oluştu", nil
}

func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return // the client is gone; nobody to answer
	}
	code, status, msg, details := classify(err)
	if status >= 500 {
		a.log.Error("request failed", "path", r.URL.Path, "code", code, "error", err)
	}
	writeError(w, status, code, msg, details)
}

func (a *api) listCampaigns(w http.ResponseWriter, r *http.Request) {
	limit := 0 // the service applies its default
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_input", "limit bir tam sayı olmalı", nil)
			return
		}
		limit = n
	}
	list, err := a.svc.List(r.Context(), limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"campaigns": list})
}
