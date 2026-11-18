package content

// JSONSchema is the structured-output schema for the message step. Length and
// variable rules cannot be expressed in the API's schema language, so Validate
// enforces them; the schema only guarantees the shape.
func JSONSchema() map[string]any {
	str := map[string]any{"type": "string"}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			FieldSMSBody:      str,
			FieldEmailSubject: str,
			FieldEmailBody:    str,
		},
		"required":             []any{FieldSMSBody, FieldEmailSubject, FieldEmailBody},
		"additionalProperties": false,
	}
}

// FieldApproach is the message step's extra field: the text's approach.
const FieldApproach = "approach"

// MessagesJSONSchema is JSONSchema plus the required approach, one of the
// fixed list (translations keep JSONSchema: a translation has no approach of
// its own).
func MessagesJSONSchema() map[string]any {
	s := JSONSchema()
	enum := make([]any, len(Approaches))
	for i, a := range Approaches {
		enum[i] = a
	}
	s["properties"].(map[string]any)[FieldApproach] = map[string]any{"type": "string", "enum": enum}
	s["required"] = append(s["required"].([]any), FieldApproach)
	return s
}
