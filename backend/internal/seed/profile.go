package seed

import (
	"math/rand/v2"
	"time"
)

// profileStream selects the random stream of the profile fields, separate from
// the one the rest of the dataset uses.
const profileStream uint64 = 0x70726f66696c65 // "profile"

// Share of each language among all users (percent). The remainder goes to the
// first entry, so the shares always add up to everyone.
var languageShares = []struct {
	code    string
	percent int
}{{"tr", 60}, {"en", 25}, {"de", 15}}

// Gender shares (percent); whoever is left did not state a gender (NULL).
var genderShares = []struct {
	value   string
	percent int
}{{"female", 48}, {"male", 48}, {"other", 2}}

const (
	noBirthDatePercent = 3  // did not state a birth date
	minAge, maxAge     = 18, 75
)

// assignProfileFields gives every user a language, and most of them a gender
// and a birth date. Each field is dealt from a shuffled list with exact counts
// (like the consents), so the mix is random per person but the totals are not
// left to chance.
func assignProfileFields(users []User, r *rand.Rand, now time.Time) {
	n := len(users)

	langs := make([]string, 0, n)
	for _, s := range languageShares[1:] {
		for k := 0; k < pct(n, s.percent); k++ {
			langs = append(langs, s.code)
		}
	}
	for len(langs) < n {
		langs = append(langs, languageShares[0].code)
	}
	r.Shuffle(n, func(i, j int) { langs[i], langs[j] = langs[j], langs[i] })

	genders := make([]*string, 0, n)
	for _, s := range genderShares {
		for k := 0; k < pct(n, s.percent); k++ {
			v := s.value
			genders = append(genders, &v)
		}
	}
	for len(genders) < n {
		genders = append(genders, nil)
	}
	r.Shuffle(n, func(i, j int) { genders[i], genders[j] = genders[j], genders[i] })

	// Birth dates: uniformly over the days between "maxAge years ago" and
	// "minAge years ago". The missing ones are chosen independently of the gender.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	oldest, youngest := today.AddDate(-maxAge, 0, 0), today.AddDate(-minAge, 0, 0)
	days := int64(youngest.Sub(oldest) / (24 * time.Hour))
	missing := make([]bool, n)
	for _, idx := range r.Perm(n)[:pct(n, noBirthDatePercent)] {
		missing[idx] = true
	}

	for i := range users {
		users[i].Language = langs[i]
		users[i].Gender = genders[i]
		if !missing[i] {
			d := oldest.AddDate(0, 0, int(r.Int64N(days+1)))
			users[i].BirthDate = &d
		}
	}
}
