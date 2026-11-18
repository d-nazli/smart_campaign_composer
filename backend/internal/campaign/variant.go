package campaign

import (
	"context"
	"crypto/md5"
	"fmt"
	"strings"

	"smartcampaign/internal/content"
	"smartcampaign/internal/segment"
)

// A/B variants (D102). Every campaign has Variant A; the marketer may add
// Variant B, a second text with the same offer, and remove it again. There is
// no third variant, no winner selection and no split setting: everyone in the
// audience gets A or B, about half each, decided by a hash of (campaign,
// person). The hash ignores the person's locale, so the person then gets their
// variant's text in their own locale.
const (
	VariantA = "A"
	VariantB = "B"
)

// Variants is every variant a campaign can have, in order.
var Variants = []string{VariantA, VariantB}

// ParseVariant reads a variant from a request: "" means A.
func ParseVariant(s string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", VariantA:
		return VariantA, nil
	case VariantB:
		return VariantB, nil
	}
	return "", &InputError{fmt.Sprintf("bilinmeyen varyant %q; yalnız A ve B var", s)}
}

// AssignVariant is the variant a person gets in a campaign: the first byte of
// md5("<campaign>:<person>"), even = A, odd = B. variantSQL computes the same
// thing in the database, where the deliveries are queued; this copy is for tests
// and for reading the rule.
func AssignVariant(campaignID, userID int64) string {
	sum := md5.Sum([]byte(fmt.Sprintf("%d:%d", campaignID, userID)))
	if sum[0]%2 == 0 {
		return VariantA
	}
	return VariantB
}

// variantSQL is AssignVariant as SQL over users.id; campaignParam is the
// placeholder of the campaign id ("$5"). md5 is used, not hashtext: md5 is
// defined, hashtext is internal and may change between Postgres versions.
func variantSQL(campaignParam string) string {
	return fmt.Sprintf(`(CASE WHEN get_byte(decode(md5((%s::bigint)::text || ':' || id::text), 'hex'), 0) %% 2 = 0 THEN 'A' ELSE 'B' END)`, campaignParam)
}

// VariantCount is how many people one variant reaches: people on at least one
// channel, and the SMS and e-mail deliveries (one per address, D79).
type VariantCount struct {
	People int `json:"people"`
	SMS    int `json:"sms"`
	Email  int `json:"email"`
}

// countByVariant counts what enqueueAudience would queue per variant, with the
// same rule, consent, contact and one-per-address filters.
func countByVariant(ctx context.Context, db queryer, c segment.Compiled, campaignID int64) (map[string]VariantCount, error) {
	args := append(append([]any{}, c.Args...), campaignID)
	v := variantSQL(fmt.Sprintf("$%d", len(args)))
	q := fmt.Sprintf(`
		WITH sms AS (SELECT DISTINCT ON (%[1]s) id FROM users WHERE %[3]s AND %[4]s ORDER BY %[1]s, id),
		     em  AS (SELECT DISTINCT ON (%[2]s) id FROM users WHERE %[3]s AND %[5]s ORDER BY %[2]s, id),
		     people AS (SELECT id FROM sms UNION SELECT id FROM em)
		SELECT v, kind, count(*) FROM (
			SELECT %[6]s AS v, 'sms' AS kind FROM sms
			UNION ALL SELECT %[6]s, 'email' FROM em
			UNION ALL SELECT %[6]s, 'people' FROM people) t
		GROUP BY 1, 2`,
		segment.SMSAddressSQL, segment.EmailAddressSQL, c.Where, segment.SMSReachableSQL, segment.EmailReachableSQL, v)
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("count by variant: %w", err)
	}
	defer rows.Close()
	out := map[string]VariantCount{VariantA: {}, VariantB: {}}
	for rows.Next() {
		var variant, kind string
		var n int
		if err := rows.Scan(&variant, &kind, &n); err != nil {
			return nil, fmt.Errorf("count by variant: %w", err)
		}
		vc := out[variant]
		switch kind {
		case "sms":
			vc.SMS = n
		case "email":
			vc.Email = n
		default:
			vc.People = n
		}
		out[variant] = vc
	}
	return out, rows.Err()
}

// sameText reports whether two texts say the same thing word for word:
// whitespace and letter case do not count. Variant B must not be Variant A
// again (an A/B test of one text measures nothing).
func sameText(a, b content.Content) bool {
	norm := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	return norm(a.SMSBody) == norm(b.SMSBody) &&
		norm(a.EmailSubject) == norm(b.EmailSubject) &&
		norm(a.EmailBody) == norm(b.EmailBody)
}

// otherVariant is B for A and A for B.
func otherVariant(v string) string {
	if v == VariantB {
		return VariantA
	}
	return VariantB
}
