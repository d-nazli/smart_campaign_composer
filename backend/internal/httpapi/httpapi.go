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
	mux.HandleFunc("POST /api/campaigns/{id}/translations", a.translate)
	mux.HandleFunc("DELETE /api/campaigns/{id}/variants/{variant}", a.deleteVariant)
	mux.HandleFunc("POST /api/campaigns/{id}/approve", a.approve)
	mux.HandleFunc("POST /api/campaigns/{id}/cancel", a.cancel)
	mux.HandleFunc("POST /api/campaigns/{id}/clarify", a.clarify)
	mux.HandleFunc("PUT /api/campaigns/{id}/brief", a.putBrief)
	mux.HandleFunc("DELETE /api/campaigns/{id}", a.deleteCampaign)
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

func (a *api) getInfo(w http.ResponseWriter, r *http.Request) {
	ls, err := a.svc.Locales(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := map[string]any{
		"llm_mode": a.info.LLMMode,
		"limits": map[string]int{
			"sms_max":        content.SMSMaxRunes,
			"subject_max":    content.SubjectMaxRunes,
			"email_body_max": content.EmailBodyMaxRunes,
		},
		"variables":     a.svc.Policy().InfosFor(ls),
		"locales":       ls.All(),
		"source_locale": ls.Source().Code,
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
	// Either one text (the source locale's: {"sms_body": ...}) or several
	// ({"locales": {"tr-TR": {...}, "en-GB": {...}}}). "variant": "B" edits
	// Variant B; without it Variant A.
	var body struct {
		content.Content
		Locales map[string]content.Content `json:"locales"`
		Variant string                     `json:"variant"`
	}
	if !decode(w, r, &body) {
		return
	}
	variant, err := campaign.ParseVariant(body.Variant)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	edits := body.Locales
	if edits != nil {
		if body.Content != (content.Content{}) {
			writeError(w, http.StatusBadRequest, "invalid_input", "ya tek metin ya da locales gönderin, ikisini birden değil", nil)
			return
		}
	} else {
		ls, err := a.svc.Locales(r.Context())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		edits = map[string]content.Content{ls.Source().Code: body.Content}
	}
	v, err := a.svc.SaveVariantContents(r.Context(), id, variant, edits)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// translate translates the source text again into some locales of the
// campaign: {"locales": ["en-GB"]}. It never takes a text: translations are
// not written by hand (D97).
func (a *api) translate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Locales []string `json:"locales"`
		Variant string   `json:"variant"`
	}
	if !decode(w, r, &body) {
		return
	}
	variant, err := campaign.ParseVariant(body.Variant)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.svc.TranslateVariant(r.Context(), id, variant, body.Locales)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// deleteVariant removes Variant B (the A/B test is turned off).
func (a *api) deleteVariant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	variant, err := campaign.ParseVariant(r.PathValue("variant"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.svc.DeleteVariant(r.Context(), id, variant)
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

// clarify answers the audience question of a campaign that waits for one; the
// audience is resolved again on the same campaign (409 if it waits for none).
func (a *api) clarify(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Answer string `json:"answer"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, err := a.svc.Clarify(r.Context(), id, body.Answer)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// putBrief replaces the brief of a campaign that was never approved and
// resolves its audience again on the same campaign (409 once approved).
func (a *api) putBrief(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Brief string `json:"brief"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, err := a.svc.UpdateBrief(r.Context(), id, body.Brief)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// deleteCampaign removes a campaign that was never approved (409 otherwise).
func (a *api) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.svc.Delete(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) streamMessages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ls, err := a.svc.Locales(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	srcCode := ls.Source().Code
	// ?variant=B writes Variant B (Variant A must exist); without it Variant A.
	variant, err := campaign.ParseVariant(r.URL.Query().Get("variant"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	s := newSSE(w, r.Context())
	defer s.close()

	err = a.svc.GenerateVariant(r.Context(), id, variant, campaign.Events{
		Attempt: func(code string, n, max int) {
			s.send("attempt", map[string]any{"locale": code, "n": n, "max": max})
		},
		Delta: func(code, field, text string) {
			s.send("delta", map[string]string{"locale": code, "field": field, "text": text})
		},
		Retry: func(code string, next int, problems []string) {
			s.send("retry", map[string]any{"locale": code, "next": next, "problems": problems})
		},
		Validated: func(code string, c content.Content, rep content.Report) {
			source := campaign.SourceTranslated
			if code == srcCode {
				source = campaign.SourceLLM
			}
			s.send("validated", map[string]any{"locale": code, "content": c, "report": rep, "source": source})
		},
		Fallback: func(code, reason string, c content.Content, rep content.Report) {
			s.send("fallback", map[string]any{"locale": code, "reason": reason, "content": c, "report": rep, "source": campaign.SourceFallback})
		},
		Failed: func(code, reason string) {
			s.send("failed", map[string]string{"locale": code, "reason": reason})
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
	s.send("done", map[string]string{"status": campaign.StatusContentReady, "variant": variant})
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
		sched   *campaign.ScheduleError
		state   *campaign.StateError
		invalid *campaign.InvalidContentError
		gen     *campaign.GenerationError
		stale   *campaign.StaleTranslationsError
		ro      *campaign.ReadOnlyTranslationError
		dup     *campaign.DuplicateVariantError
	)
	switch {
	case errors.Is(err, campaign.ErrNotFound):
		return "not_found", http.StatusNotFound, "kampanya bulunamadı", nil
	case errors.As(err, &input):
		return "invalid_input", http.StatusBadRequest, input.Message, nil
	case errors.As(err, &sched):
		return "invalid_schedule", http.StatusUnprocessableEntity, sched.Message, nil
	case errors.As(err, &stale):
		details := map[string]any{"missing": nonNil(stale.Missing), "stale": nonNil(stale.Stale)}
		if len(stale.Variants) > 0 {
			details["variants"] = stale.Variants
		}
		return "stale_translations", http.StatusConflict, "güncel olmayan veya eksik çeviri var; önce çevrilmeli", details
	case errors.As(err, &dup):
		return "variant_duplicate", http.StatusUnprocessableEntity, dup.Error(), nil
	case errors.As(err, &ro):
		return "translation_read_only", http.StatusUnprocessableEntity, ro.Error(), map[string]any{"locale": ro.Locale}
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

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
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
