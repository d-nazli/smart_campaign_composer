// Package seed generates the deterministic fake dataset (users, cart events,
// orders) and loads it into Postgres idempotently.
package seed

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultUsers is the dataset size used by docker compose.
	DefaultUsers = 5000

	// BriefCity and BriefWindowDays describe the example brief from CASE.md:
	// "cart add in the last 30 days, no purchase since, in Istanbul".
	BriefCity       = "İstanbul"
	BriefWindowDays = 30

	minUsers = 200
)

// User mirrors the users table. IDs are explicit (1..n) so the dataset is
// reproducible byte for byte.
type User struct {
	ID              int64
	FirstName       string
	LastName        string
	Email           *string
	Phone           *string
	City            string
	SMSConsent      bool
	EmailConsent    bool
	LastCartAddAt   *time.Time
	LastOrderAt     *time.Time
	OrderCount      int
	TotalSpentCents int64
	CreatedAt       time.Time
}

type CartEvent struct {
	ID      int64
	UserID  int64
	Product string
	AddedAt time.Time
}

type Order struct {
	ID         int64
	UserID     int64
	TotalCents int64
	CreatedAt  time.Time
}

type Dataset struct {
	Users      []User
	CartEvents []CartEvent
	Orders     []Order
}

// InBriefCohort reports whether u matches the example brief at time now.
func (u User) InBriefCohort(now time.Time) bool {
	if u.City != BriefCity || u.LastCartAddAt == nil {
		return false
	}
	if now.Sub(*u.LastCartAddAt) > BriefWindowDays*24*time.Hour {
		return false
	}
	// "No purchase since the cart add".
	return u.LastOrderAt == nil || u.LastOrderAt.Before(*u.LastCartAddAt)
}

func (u User) SMSReachable() bool   { return u.SMSConsent && u.Phone != nil }
func (u User) EmailReachable() bool { return u.EmailConsent && u.Email != nil }

// CohortCount counts users matching the example brief.
func (d *Dataset) CohortCount(now time.Time) int {
	n := 0
	for _, u := range d.Users {
		if u.InBriefCohort(now) {
			n++
		}
	}
	return n
}

type profile int

const (
	profNeutral      profile = iota // random behaviour that never matches the brief
	profCohort                      // Istanbul, recent cart add, no purchase since
	profDecoyOld                    // Istanbul, last cart add too old (36+ days)
	profDecoyOrdered                // Istanbul, recent cart add but purchased afterwards
	profDecoyCity                   // recent cart add, no purchase, other city
)

// Generate builds the dataset. It is a pure function of (seed, now, n):
// same inputs give identical output. All dates are relative to now.
//
// Dates are deliberately never placed in the 26..35 day band, so the cohort
// does not depend on the time of day the seed runs.
func Generate(seed uint64, now time.Time, n int) (*Dataset, error) {
	if n < minUsers {
		return nil, fmt.Errorf("seed: need at least %d users, got %d", minUsers, n)
	}
	now = now.UTC().Truncate(time.Second)
	g := &gen{r: rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15)), now: now}

	users := make([]User, n)
	for i := range users {
		users[i].ID = int64(i + 1)
	}

	g.assignCities(users)
	g.assignConsents(users)
	g.assignNames(users)
	g.assignContacts(users)

	profiles := g.assignProfiles(users)
	ds := &Dataset{Users: users}
	for i := range users {
		g.addActivity(ds, i, profiles[i])
	}
	g.deriveColumns(ds)

	for i := range users {
		users[i].CreatedAt = g.ago(1, 730)
	}
	return ds, nil
}

type gen struct {
	r   *rand.Rand
	now time.Time

	nextCartID  int64
	nextOrderID int64
}

// ago returns a random instant in [minDays, maxDays+1) days before now.
func (g *gen) ago(minDays, maxDays int) time.Time {
	span := int64(maxDays-minDays+1) * 24 * 3600
	secs := int64(minDays)*24*3600 + g.r.Int64N(span)
	return g.now.Add(-time.Duration(secs) * time.Second)
}

func (g *gen) recentCart() time.Time { return g.ago(1, 25) }   // inside the 30-day window
func (g *gen) oldCart() time.Time    { return g.ago(36, 120) } // outside the 30-day window

func pct(n, p int) int { return n * p / 100 }

func (g *gen) assignCities(users []User) {
	n := len(users)
	cities := make([]string, 0, n)
	for _, c := range cityShares {
		for k := 0; k < pct(n, c.percent); k++ {
			cities = append(cities, c.name)
		}
	}
	for len(cities) < n { // rounding remainder
		cities = append(cities, BriefCity)
	}
	g.r.Shuffle(n, func(i, j int) { cities[i], cities[j] = cities[j], cities[i] })
	for i := range users {
		users[i].City = cities[i]
	}
}

func (g *gen) assignConsents(users []User) {
	n := len(users)
	// 60% both, 12% SMS only, 18% email only, remaining 10% none.
	kinds := make([]uint8, 0, n) // bit0 = sms, bit1 = email
	add := func(count int, kind uint8) {
		for k := 0; k < count; k++ {
			kinds = append(kinds, kind)
		}
	}
	add(pct(n, 60), 3)
	add(pct(n, 12), 1)
	add(pct(n, 18), 2)
	add(n-len(kinds), 0)
	g.r.Shuffle(n, func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
	for i := range users {
		users[i].SMSConsent = kinds[i]&1 != 0
		users[i].EmailConsent = kinds[i]&2 != 0
	}
}

func (g *gen) assignNames(users []User) {
	for i := range users {
		users[i].FirstName = firstNames[g.r.IntN(len(firstNames))]
		users[i].LastName = lastNames[g.r.IntN(len(lastNames))]
	}
	// 0.6% (30 of 5000) get a 30..45 rune first name to exercise length limits.
	perm := g.r.Perm(len(users))
	for _, idx := range perm[:len(users)*6/1000] {
		users[idx].FirstName = g.longFirstName()
	}
}

func (g *gen) longFirstName() string {
	var parts []string
	for utf8.RuneCountInString(strings.Join(parts, " ")) < 30 {
		parts = append(parts, longNameParts[g.r.IntN(len(longNameParts))])
	}
	return strings.Join(parts, " ")
}

var turkishFold = strings.NewReplacer(
	"ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
	"Ç", "c", "Ğ", "g", "İ", "i", "I", "i", "Ö", "o", "Ş", "s", "Ü", "u",
)

func asciiLower(s string) string {
	s = turkishFold.Replace(s)
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (g *gen) assignContacts(users []User) {
	n := len(users)
	nNull := pct(n, 3)
	nBad := n * 5 / 1000 // 0.5%

	for i := range users {
		u := &users[i]
		email := fmt.Sprintf("%s.%s.%d@example.com", asciiLower(firstWord(u.FirstName)), asciiLower(u.LastName), u.ID)
		phone := fmt.Sprintf("+90555%07d", u.ID)
		u.Email, u.Phone = &email, &phone
	}
	// Phones: nNull missing, then nBad malformed (disjoint sets).
	perm := g.r.Perm(n)
	for _, idx := range perm[:nNull] {
		users[idx].Phone = nil
	}
	for k, idx := range perm[nNull : nNull+nBad] {
		v := invalidPhones[k%len(invalidPhones)]
		users[idx].Phone = &v
	}
	// Emails: independent permutation, same sizes.
	perm = g.r.Perm(n)
	for _, idx := range perm[:nNull] {
		users[idx].Email = nil
	}
	for k, idx := range perm[nNull : nNull+nBad] {
		v := invalidEmails[k%len(invalidEmails)]
		users[idx].Email = &v
	}
}

func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}

// assignProfiles picks which users form the brief cohort and the decoy groups.
func (g *gen) assignProfiles(users []User) []profile {
	n := len(users)
	profiles := make([]profile, n)

	var istanbul, others []int
	for i, u := range users {
		if u.City == BriefCity {
			istanbul = append(istanbul, i)
		} else {
			others = append(others, i)
		}
	}
	g.r.Shuffle(len(istanbul), func(i, j int) { istanbul[i], istanbul[j] = istanbul[j], istanbul[i] })
	g.r.Shuffle(len(others), func(i, j int) { others[i], others[j] = others[j], others[i] })

	nCohort, nDecoy := pct(n, 6), pct(n, 3) // 300 and 150 of 5000
	take := func(src []int, count int, p profile) []int {
		for _, idx := range src[:count] {
			profiles[idx] = p
		}
		return src[count:]
	}
	istanbul = take(istanbul, nCohort, profCohort)
	istanbul = take(istanbul, nDecoy, profDecoyOld)
	_ = take(istanbul, nDecoy, profDecoyOrdered)
	_ = take(others, nDecoy, profDecoyCity)
	return profiles
}

func (g *gen) cart(userID int64, at time.Time) CartEvent {
	g.nextCartID++
	return CartEvent{ID: g.nextCartID, UserID: userID, Product: products[g.r.IntN(len(products))], AddedAt: at}
}

func (g *gen) order(userID int64, at time.Time) Order {
	g.nextOrderID++
	return Order{ID: g.nextOrderID, UserID: userID, TotalCents: 5000 + g.r.Int64N(295001), CreatedAt: at}
}

// orderAfter returns an order strictly after `after` (at least one hour) and before now.
func (g *gen) orderAfter(userID int64, after time.Time) Order {
	room := int64(g.now.Sub(after)/time.Second) - 2*3600
	if room < 1 {
		room = 1
	}
	return g.order(userID, after.Add(time.Hour+time.Duration(g.r.Int64N(room))*time.Second))
}

func (g *gen) addActivity(ds *Dataset, idx int, p profile) {
	uid := ds.Users[idx].ID
	addCart := func(at time.Time) { ds.CartEvents = append(ds.CartEvents, g.cart(uid, at)) }
	var mine []Order // this user's orders only
	addOrder := func(o Order) {
		ds.Orders = append(ds.Orders, o)
		mine = append(mine, o)
	}
	randomOrders := func(minDays int) {
		for k, c := 0, 1+g.r.IntN(8); k < c; k++ {
			addOrder(g.order(uid, g.ago(minDays, 365)))
		}
	}

	switch p {
	case profCohort, profDecoyCity:
		last := g.recentCart()
		addCart(last)
		for k, c := 0, g.r.IntN(3); k < c; k++ {
			addCart(g.oldCart())
		}
		// Any earlier orders are 40+ days old, i.e. before the last cart add.
		if g.r.Float64() < 0.4 {
			for k, c := 0, 1+g.r.IntN(3); k < c; k++ {
				addOrder(g.order(uid, g.ago(40, 365)))
			}
		}

	case profDecoyOld:
		for k, c := 0, 1+g.r.IntN(3); k < c; k++ {
			addCart(g.oldCart())
		}
		if g.r.Float64() < 0.5 {
			randomOrders(1)
		}

	case profDecoyOrdered:
		last := g.recentCart()
		addCart(last)
		addOrder(g.orderAfter(uid, last))
		if g.r.Float64() < 0.3 {
			addOrder(g.order(uid, g.ago(40, 365)))
		}

	default: // profNeutral
		// 35% no activity, 25% orders only, 15% carts only, 25% both.
		kind := g.r.Float64()
		if kind < 0.35 {
			return
		}
		hasCarts := kind >= 0.60
		hasOrders := kind < 0.60 || kind >= 0.75
		var lastCart *time.Time
		if hasCarts {
			for k, c := 0, 1+g.r.IntN(4); k < c; k++ {
				var at time.Time
				if g.r.Float64() < 0.35 {
					at = g.recentCart()
				} else {
					at = g.oldCart()
				}
				addCart(at)
				if lastCart == nil || at.After(*lastCart) {
					t := at
					lastCart = &t
				}
			}
		}
		if hasOrders {
			randomOrders(1)
		}
		// Keep random Istanbul users out of the brief cohort: if the latest
		// cart add is recent and nothing was bought after it, add an order.
		if ds.Users[idx].City == BriefCity && lastCart != nil && g.now.Sub(*lastCart) <= BriefWindowDays*24*time.Hour {
			bought := false
			for _, o := range mine {
				if o.CreatedAt.After(*lastCart) {
					bought = true
					break
				}
			}
			if !bought {
				addOrder(g.orderAfter(uid, *lastCart))
			}
		}
	}
}

// deriveColumns fills last_cart_add_at, last_order_at, order_count and
// total_spent from the raw events.
func (g *gen) deriveColumns(ds *Dataset) {
	byID := make(map[int64]*User, len(ds.Users))
	for i := range ds.Users {
		byID[ds.Users[i].ID] = &ds.Users[i]
	}
	for _, c := range ds.CartEvents {
		u := byID[c.UserID]
		if u.LastCartAddAt == nil || c.AddedAt.After(*u.LastCartAddAt) {
			t := c.AddedAt
			u.LastCartAddAt = &t
		}
	}
	for _, o := range ds.Orders {
		u := byID[o.UserID]
		if u.LastOrderAt == nil || o.CreatedAt.After(*u.LastOrderAt) {
			t := o.CreatedAt
			u.LastOrderAt = &t
		}
		u.OrderCount++
		u.TotalSpentCents += o.TotalCents
	}
}
