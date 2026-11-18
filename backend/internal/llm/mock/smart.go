package mock

import (
	"encoding/json"

	"smartcampaign/internal/brief"
)

// smartSegment answers the way a flawless model would: it uses the same brief
// reader the backend verifies the real model against. The mock is a demo and
// test double, so it is deterministic; the interesting failure paths (a model
// that drops a condition or the offer) are scripted in tests with Raw steps.
// The messages are written by the copywriter in copy.go.
func smartSegment(text string) string {
	rule := brief.Analyze(text).Rule()
	// The one-click suggestion and the note are decided by code, never by a
	// model, and the backend overwrites them; the mock imitates a model.
	rule.Suggestion, rule.Note = "", ""
	rule.Name = brief.SuggestName(text, rule)
	raw, _ := json.Marshal(rule)
	return string(raw)
}
