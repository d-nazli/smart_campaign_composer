package segment

import "smartcampaign/internal/offer"

// JSONSchema is the structured-output schema sent to the LLM. It is derived
// from the field whitelist. The API's schema language cannot express ranges,
// lengths or per-field value types, so Parse re-checks everything; the schema
// only makes malformed JSON very unlikely.
func JSONSchema() map[string]any {
	fieldNames := make([]any, len(fields))
	opSet := map[string]bool{}
	var ops []any
	for i, f := range fields {
		fieldNames[i] = f.Name
		for _, o := range f.Ops {
			if !opSet[o] {
				opSet[o] = true
				ops = append(ops, o)
			}
		}
	}
	str := map[string]any{"type": "string"}
	condition := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"field": map[string]any{"type": "string", "enum": fieldNames},
			"op":    map[string]any{"type": "string", "enum": ops},
			"value": map[string]any{"anyOf": []any{
				str,
				map[string]any{"type": "number"},
				map[string]any{"type": "boolean"},
				map[string]any{"type": "array", "items": str},
			}},
		},
		"required":             []any{"field", "op", "value"},
		"additionalProperties": false,
	}
	kinds := make([]any, len(offer.Kinds))
	for i, k := range offer.Kinds {
		kinds[i] = string(k)
	}
	partKinds := make([]any, len(offer.PartKinds))
	for i, k := range offer.PartKinds {
		partKinds[i] = string(k)
	}
	part := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind":  map[string]any{"type": "string", "enum": partKinds},
			"value": map[string]any{"type": "number"},
		},
		"required":             []any{"kind", "value"},
		"additionalProperties": false,
	}
	offerSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind":         map[string]any{"type": "string", "enum": kinds},
			"value":        map[string]any{"type": "number"},
			"code":         str,
			"min_spend":    map[string]any{"type": "number"}, // 0 = no minimum basket
			"expires_at":   str,                              // YYYY-MM-DD or ""
			"step_spend":   map[string]any{"type": "number"}, // 0 = not tiered
			"max_discount": map[string]any{"type": "number"}, // 0 = no cap
			"extras":       map[string]any{"type": "array", "items": part},
		},
		"required":             []any{"kind", "value", "code", "min_spend", "expires_at", "step_spend", "max_discount", "extras"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        str,
			"status":      map[string]any{"type": "string", "enum": []any{StatusOK, StatusNeedsClarification, StatusUnsupported}},
			"match":       map[string]any{"type": "string", "enum": []any{MatchAll, MatchAny}},
			"conditions":  map[string]any{"type": "array", "items": condition},
			"explanation": str,
			"question":    str,
			"offer":       offerSchema,
		},
		"required":             []any{"name", "status", "match", "conditions", "explanation", "question", "offer"},
		"additionalProperties": false,
	}
}
