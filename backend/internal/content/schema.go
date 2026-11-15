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
