package mock

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"smartcampaign/internal/segment"
)

var (
	unsupportedRe = regexp.MustCompile(`\b(yas|yasindaki|cinsiyet|dogum|kadin|kadinlara|erkek|erkeklere)\b`)
	windowRe      = regexp.MustCompile(`(?:son\s+)?(\d+)\s*gun`)
	notBoughtRe   = regexp.MustCompile(`(satin\s*alma(yan|mis)|alisveris\s*yapma(yan|mis)|siparis\s*verme(yen|mis)|almayan|yapmayan|vermeyen)`)
	boughtRe      = regexp.MustCompile(`(satin\s*alan|alisveris\s*yapan|siparis\s*veren)`)
	discountRe    = regexp.MustCompile(`(?:%\s*(\d{1,2})|(\d{1,2})\s*%)`)
)

type cond struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type segmentJSON struct {
	Status      string `json:"status"`
	Match       string `json:"match"`
	Conditions  []cond `json:"conditions"`
	Explanation string `json:"explanation"`
	Question    string `json:"question"`
}

// smartSegment reads a Turkish brief with keyword rules. It is intentionally
// simple: its job is to give a believable demo and to be deterministic.
func smartSegment(brief string) string {
	b := segment.Fold(brief)
	out := segmentJSON{Status: segment.StatusOK, Match: segment.MatchAll, Conditions: []cond{}}
	finish := func() string {
		raw, _ := json.Marshal(out)
		return string(raw)
	}

	if unsupportedRe.MatchString(b) {
		out.Status, out.Explanation = segment.StatusUnsupported,
			"Brief, veritabanında bulunmayan bir müşteri bilgisine (yaş, cinsiyet gibi) dayanıyor; şehir, sepet ve sipariş geçmişiyle ifade edilebilen bir kitle tanımlayın."
		return finish()
	}

	var cities []string
	for _, c := range segment.KnownCities {
		if strings.Contains(b, segment.Fold(c)) {
			cities = append(cities, c)
		}
	}

	var parts []string // pieces of the explanation
	switch len(cities) {
	case 0:
	case 1:
		out.Conditions = append(out.Conditions, cond{"city", segment.OpEq, cities[0]})
		parts = append(parts, cities[0]+"'daki müşteriler")
	default:
		out.Conditions = append(out.Conditions, cond{"city", segment.OpIn, cities})
		parts = append(parts, strings.Join(cities, ", ")+" şehirlerindeki müşteriler")
	}

	days := -1
	if m := windowRe.FindStringSubmatch(b); m != nil {
		days, _ = strconv.Atoi(m[1])
		if days > 3650 {
			days = 3650
		}
	}
	notBought, bought := notBoughtRe.MatchString(b), boughtRe.MatchString(b)

	if strings.Contains(b, "sepet") {
		if days >= 0 {
			out.Conditions = append(out.Conditions, cond{"days_since_last_cart_add", segment.OpLte, days})
			parts = append(parts, fmt.Sprintf("son %d günde sepete ürün ekleyen", days))
		} else {
			out.Conditions = append(out.Conditions, cond{"days_since_last_cart_add", segment.OpGte, 0})
			parts = append(parts, "sepete ürün eklemiş")
		}
		if notBought {
			out.Conditions = append(out.Conditions, cond{"purchased_since_last_cart_add", segment.OpEq, false})
			parts = append(parts, "sonrasında satın almamış")
		} else if bought {
			out.Conditions = append(out.Conditions, cond{"purchased_since_last_cart_add", segment.OpEq, true})
			parts = append(parts, "sonrasında satın almış")
		}
	} else {
		switch {
		case notBought:
			out.Conditions = append(out.Conditions, cond{"order_count", segment.OpEq, 0})
			parts = append(parts, "hiç sipariş vermemiş")
		case bought && days >= 0:
			out.Conditions = append(out.Conditions, cond{"days_since_last_order", segment.OpLte, days})
			parts = append(parts, fmt.Sprintf("son %d günde sipariş veren", days))
		case bought:
			out.Conditions = append(out.Conditions, cond{"order_count", segment.OpGt, 0})
			parts = append(parts, "en az bir sipariş vermiş")
		}
	}

	if len(out.Conditions) == 0 {
		out.Status = segment.StatusNeedsClarification
		out.Question = "Mesajı kimlere göndermek istiyorsunuz? Örneğin bir şehir, sepet davranışı (sepete ekleyip satın almayanlar) veya sipariş geçmişi belirtebilirsiniz."
		return finish()
	}
	out.Explanation = strings.Join(parts, ", ")
	return finish()
}

type messagesJSON struct {
	SMSBody      string `json:"sms_body"`
	EmailSubject string `json:"email_subject"`
	EmailBody    string `json:"email_body"`
}

// smartMessages writes believable Turkish copy that follows the brief's tone
// and discount, always inside the SMS/subject limits.
func smartMessages(brief string) string {
	b := segment.Fold(brief)
	informal := strings.Contains(b, "samimi") || strings.Contains(b, "sen ")
	short := strings.Contains(b, "kisa")
	cart := strings.Contains(b, "sepet")

	discount := ""
	if m := discountRe.FindStringSubmatch(b); m != nil {
		d := m[1]
		if d == "" {
			d = m[2]
		}
		discount = "%" + d + " indirim"
	}

	var m messagesJSON
	switch {
	case cart && informal:
		offer := "özel fırsatı"
		if discount != "" {
			offer = discount + " fırsatını"
		}
		m.SMSBody = fmt.Sprintf("{{first_name}}, sepetindeki ürünler seni bekliyor! %s kaçırma.", capitalizeFirst(offer))
		m.EmailSubject = "{{first_name}}, sepetindekiler seni bekliyor"
		if discount != "" {
			m.EmailSubject = fmt.Sprintf("{{first_name}}, sepetindekiler %s", discount+"de")
		}
		m.EmailBody = "Merhaba {{first_name}},\n\nSepetine eklediğin ürünler hâlâ seni bekliyor. " + offerLine(discount, true) +
			"\n\nAlışverişini tamamlamak için sepetine göz atman yeterli.\n\nGörüşmek üzere!"
	case cart:
		m.SMSBody = "Sayın {{first_name}}, sepetinizdeki ürünler sizi bekliyor. " + offerLineSMS(discount) + " fırsatından yararlanın."
		m.EmailSubject = "Sepetinizdeki ürünler sizi bekliyor"
		m.EmailBody = "Sayın {{first_name}},\n\nSepetinize eklediğiniz ürünler hâlâ sizi bekliyor. " + offerLine(discount, false) +
			"\n\nAlışverişinizi tamamlamak için sepetinize göz atabilirsiniz.\n\nSaygılarımızla"
	case informal:
		m.SMSBody = "{{first_name}}, sana özel " + offerLineSMS(discount) + " seni bekliyor!"
		m.EmailSubject = "{{first_name}}, sana özel bir fırsat var"
		m.EmailBody = "Merhaba {{first_name}},\n\nSana özel bir kampanyamız var. " + offerLine(discount, true) + "\n\nGörüşmek üzere!"
	default:
		m.SMSBody = "Sayın {{first_name}}, size özel " + offerLineSMS(discount) + " sizi bekliyor."
		m.EmailSubject = "Size özel bir fırsat var"
		m.EmailBody = "Sayın {{first_name}},\n\nSize özel bir kampanyamız var. " + offerLine(discount, false) + "\n\nSaygılarımızla"
	}
	if short {
		if i := strings.Index(m.EmailBody, "\n\nAlışverişini"); i > 0 {
			m.EmailBody = m.EmailBody[:i] + "\n\nGörüşmek üzere!"
		}
		if i := strings.Index(m.EmailBody, "\n\nAlışverişinizi"); i > 0 {
			m.EmailBody = m.EmailBody[:i] + "\n\nSaygılarımızla"
		}
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

func offerLine(discount string, informal bool) string {
	switch {
	case discount != "" && informal:
		return discount + " fırsatını kaçırma."
	case discount != "":
		return discount + " fırsatını kaçırmayın."
	case informal:
		return "Fırsatı kaçırma."
	}
	return "Fırsatı kaçırmayın."
}

func offerLineSMS(discount string) string {
	if discount == "" {
		return "özel fırsat"
	}
	return discount
}

func capitalizeFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
