package content

// Fallback returns the built-in template used when the LLM could not produce
// valid content. It is itself validated against the policy: it can never break
// the SMS/subject limits, even if budgets are configured unusually large; in
// that case a variable-free template is returned instead.
func (p Policy) Fallback() Content {
	personal := Content{
		SMSBody:      "Merhaba {{first_name}}, sana özel bir fırsatımız var! Detaylar için bizi ziyaret et.",
		EmailSubject: "{{first_name}}, sana özel bir fırsat var",
		EmailBody: "Merhaba {{first_name}},\n\n" +
			"Sana özel bir kampanyamız var. Fırsatı kaçırmamak için hemen göz at.\n\n" +
			"Sevgiler",
	}
	if norm, rep := p.Validate(personal); rep.OK {
		return norm
	}
	plain := Content{
		SMSBody:      "Size özel bir fırsatımız var! Detaylar için bizi ziyaret edin.",
		EmailSubject: "Size özel bir fırsat var",
		EmailBody:    "Merhaba,\n\nSize özel bir kampanyamız var. Fırsatı kaçırmamak için hemen göz atın.\n\nSevgiler",
	}
	norm, _ := p.Validate(plain)
	return norm
}
