// Package locale reads the locales table. It is the only place the list of
// locales comes from: adding a locale is adding a row, no code changes.
package locale

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"smartcampaign/internal/content"
)

// Querier is satisfied by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Load reads every locale and checks it against the policy (NewLocales).
func Load(ctx context.Context, q Querier, p content.Policy) (content.Locales, error) {
	rows, err := q.Query(ctx, `
		SELECT code, name, native_name, direction, is_source, fallbacks, free_shipping_terms
		FROM locales ORDER BY code`)
	if err != nil {
		return content.Locales{}, fmt.Errorf("load locales: %w", err)
	}
	defer rows.Close()
	var list []content.Locale
	for rows.Next() {
		var l content.Locale
		var fallbacks []byte
		if err := rows.Scan(&l.Code, &l.Name, &l.NativeName, &l.Direction, &l.IsSource, &fallbacks, &l.FreeShippingTerms); err != nil {
			return content.Locales{}, fmt.Errorf("load locales: %w", err)
		}
		if err := json.Unmarshal(fallbacks, &l.Fallbacks); err != nil {
			return content.Locales{}, fmt.Errorf("locale %s: fallbacks: %w", l.Code, err)
		}
		list = append(list, l)
	}
	if err := rows.Err(); err != nil {
		return content.Locales{}, fmt.Errorf("load locales: %w", err)
	}
	ls, err := content.NewLocales(list, p)
	if err != nil {
		return content.Locales{}, fmt.Errorf("locales table: %w", err)
	}
	return ls, nil
}
