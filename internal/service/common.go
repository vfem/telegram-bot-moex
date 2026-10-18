package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
)

type sfCall[T any] struct {
	wg  sync.WaitGroup
	val T
	err error
}

// singleflightGroup deduplicates concurrent in-flight requests for the same key.
type singleflightGroup[T any] struct {
	mu sync.Mutex
	m  map[string]*sfCall[T]
}

func (g *singleflightGroup[T]) Do(key string, fn func() (T, error)) (T, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*sfCall[T])
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(sfCall[T])
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}

// resolveBondByIdentifier resolves a bond by ISIN or Ticker with SearchBonds fallback.
func resolveBondByIdentifier(ctx context.Context, bp provider.BondProvider, id, rawIdentifier string) (*domain.Bond, error) {
	if bp == nil {
		return nil, fmt.Errorf("%w: %s", ErrBondNotFound, rawIdentifier)
	}
	bond, err := bp.GetBond(ctx, id)
	if err == nil && bond != nil {
		return bond, nil
	}

	results, searchErr := bp.SearchBonds(ctx, id)
	if searchErr != nil || len(results) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrBondNotFound, rawIdentifier)
	}
	var matchedISIN string
	for _, r := range results {
		if strings.EqualFold(r.Ticker, id) || strings.EqualFold(r.ISIN, id) {
			matchedISIN = r.ISIN
			break
		}
	}
	if matchedISIN == "" {
		if len(results) == 1 {
			matchedISIN = results[0].ISIN
		} else {
			return nil, fmt.Errorf("%w: неоднозначный тикер %s", ErrBondNotFound, rawIdentifier)
		}
	}
	bond, err = bp.GetBond(ctx, matchedISIN)
	if err != nil || bond == nil {
		return nil, fmt.Errorf("%w: %s", ErrBondNotFound, rawIdentifier)
	}
	return bond, nil
}

// isFederalSovereignBond returns true if the bond is issued by the federal Ministry of Finance (OFZ).
func isFederalSovereignBond(bond *domain.Bond) bool {
	if bond == nil {
		return false
	}
	issuer := strings.TrimSpace(bond.Issuer)
	return bond.IssuerINN == sovereignINN ||
		strings.EqualFold(issuer, "Министерство финансов Российской Федерации") ||
		strings.EqualFold(issuer, "Министерство финансов РФ") ||
		strings.EqualFold(issuer, "Минфин РФ") ||
		strings.EqualFold(issuer, "Минфин России")
}
