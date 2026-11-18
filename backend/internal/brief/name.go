package brief

import (
	"strings"
	"unicode/utf8"

	"smartcampaign/internal/llm"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// topicNames is how a topic reads in a campaign name.
var topicNames = map[string]string{
	llm.TopicCart:    "sepet hatırlatma",
	llm.TopicWinback: "geri kazanım",
	llm.TopicWelcome: "hoş geldin",
	llm.TopicLoyal:   "sadık müşteri",
	llm.TopicGeneral: "kampanya",
}

// SuggestName writes a short campaign name from the brief and its rule, the way
// the mock "model" names a campaign: the city (when the audience is one city)
// or the special day, the topic, and the offer, for example
// "İstanbul sepet hatırlatma · %30". It fits segment.MaxNameRunes.
func SuggestName(text string, r segment.Rule) string {
	th := ThemeOf(text, "tr")
	head := topicNames[th.Topic]
	if head == "" {
		head = topicNames[llm.TopicGeneral]
	}
	if th.OccasionName != "" {
		head = th.OccasionName
	} else if city := singleCity(r); city != "" {
		if th.Topic == llm.TopicGeneral || topicNames[th.Topic] == "" {
			head = city + " kampanyası"
		} else {
			head = city + " " + head
		}
	}
	name := upperFirstTR(head)
	if o := shortOffer(r.Offer.Normalize()); o != "" {
		name += " · " + o
	}
	return clipName(name, segment.MaxNameRunes)
}

func singleCity(r segment.Rule) string {
	for _, c := range r.Conditions {
		if c.Field == "city" && c.Op == segment.OpEq {
			if s, ok := c.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

// shortOffer is the main part of the offer in a few characters ("%30", "100 TL", "ücretsiz kargo").
func shortOffer(o offer.Offer) string {
	switch o.Kind {
	case offer.KindPercent:
		return "%" + offer.FormatNumber(o.Value)
	case offer.KindAmount:
		return offer.FormatNumber(o.Value) + " TL"
	case offer.KindFreeShipping:
		return "ücretsiz kargo"
	case offer.KindCouponCode:
		return "kupon " + o.Code
	}
	return ""
}

func upperFirstTR(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	switch r {
	case 'i':
		return "İ" + s[size:]
	case 'ı':
		return "I" + s[size:]
	}
	return strings.ToUpper(string(r)) + s[size:]
}

// clipName cuts a name at a word boundary so it fits max characters.
func clipName(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	out := ""
	for _, w := range strings.Fields(s) {
		next := strings.TrimSpace(out + " " + w)
		if utf8.RuneCountInString(next) > max {
			break
		}
		out = next
	}
	return strings.TrimRight(out, " ·")
}
