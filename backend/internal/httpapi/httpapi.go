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

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/httpx"
)

const maxBodyBytes = 32 << 10

// Info is what the UI needs to know about the running configuration.
type Info struct {
	LLMMode string
	Model   string // only reported for the real model
}

// New builds the router. ready is the database readiness check.
func New(svc *campaign.Service, info Info, ready func(context.Context) error, log *slog.Logger) http.Handler {
	a := &api{svc: svc, info: info, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Healthz)
	mux.HandleFunc("GET /readyz", httpx.Readyz(ready))
	mux.HandleFunc("GET /api/info", a.getInfo)
	mux.HandleFunc("POST /api/campaigns", a.createCampaign)
	mux.HandleFunc("GET /api/campaigns/{id}", a.getCampaign)
	mux.HandleFunc("POST /api/campaigns/{id}/messages/stream", a.streamMessages)
	mux.HandleFunc("PUT /api/campaigns/{id}/content", a.putContent)
	mux.HandleFunc("POST /api/campaigns/{id}/approve", a.approve)
	return mux
}

type api struct {
	svc  *campaign.Service
	info Info
	log  *slog.Logger
}

func (a *api) getInfo(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{
		"llm_mode": a.info.LLMMode,
		"limits": map[string]int{
			"sms_max":        content.SMSMaxRunes,
			"subject_max":    content.SubjectMaxRunes,
			"email_body_max": content.EmailBodyMaxRunes,
		},
		"variables": a.svc.Policy().Infos(),
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
	var c content.Content
	if !decode(w, r, &c) {
		return
	}
	v, err := a.svc.SaveContent(r.Context(), id, c)
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
	res, err := a.svc.Approve(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

func (a *api) streamMessages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s := newSSE(w, r.Context())
	defer s.close()

	err := a.svc.GenerateMessages(r.Context(), id, campaign.Events{
		Attempt: func(n, max int) { s.send("attempt", map[string]int{"n": n, "max": max}) },
		Delta:   func(field, text string) { s.send("delta", map[string]string{"field": field, "text": text}) },
		Retry: func(next int, problems []string) {
			s.send("retry", map[string]any{"next": next, "problems": problems})
		},
		Validated: func(c content.Content, rep content.Report) {
			s.send("validated", map[string]any{"content": c, "report": rep, "source": campaign.SourceLLM})
		},
		Fallback: func(reason string, c content.Content, rep content.Report) {
			s.send("fallback", map[string]any{"reason": reason, "content": c, "report": rep, "source": campaign.SourceFallback})
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
