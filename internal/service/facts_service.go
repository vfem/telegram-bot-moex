package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
	"telegram-bot-moex/internal/provider/edisclosure"
	"telegram-bot-moex/internal/store"
)

// ErrNoMaterialFacts indicates that no material facts were found for the issuer.
var ErrNoMaterialFacts = errors.New("существенные факты не найдены")

const negativeFactsCacheTTL = 15 * time.Minute

// FactsResult bundles the resolved issuer metadata and its material facts.
type FactsResult struct {
	INN         string
	ISIN        string
	IssuerName  string
	IsSovereign bool
	Facts       []domain.MaterialFact
}

// FactsService coordinates corporate disclosure lookups, classification, and caching.
type FactsService struct {
	bondProvider  provider.BondProvider
	factsProvider provider.FactsProvider
	cache         store.FactsCache
	sf            singleflightGroup[[]domain.MaterialFact]
}

// NewFactsService creates a new FactsService.
func NewFactsService(bp provider.BondProvider, fp provider.FactsProvider, cache store.FactsCache) *FactsService {
	return &FactsService{
		bondProvider:  bp,
		factsProvider: fp,
		cache:         cache,
	}
}

// GetIssuerFacts retrieves the most recent material facts by ISIN, Ticker, or INN.
func (s *FactsService) GetIssuerFacts(ctx context.Context, identifier string, limit int) (*FactsResult, error) {
	id := strings.TrimSpace(strings.ToUpper(identifier))
	if id == "" {
		return nil, ErrInvalidIdentifier
	}
	if limit <= 0 {
		limit = 5
	}

	isINN := innRegex.MatchString(id)
	isISIN := isinRegex.MatchString(id)
	isTicker := tickerRegex.MatchString(id)

	if !isINN && !isISIN && !isTicker {
		return nil, ErrInvalidIdentifier
	}

	if isINN {
		if !domain.IsValidINN(id) {
			return nil, ErrInvalidINNChecksum
		}
		if id == sovereignINN {
			return &FactsResult{
				INN:         sovereignINN,
				IssuerName:  "Министерство финансов Российской Федерации",
				IsSovereign: true,
			}, nil
		}
		return s.getFactsByINN(ctx, id, "", "", limit)
	}

	// Resolve bond by ISIN or Ticker
	bond, err := resolveBondByIdentifier(ctx, s.bondProvider, id, identifier)
	if err != nil {
		return nil, err
	}

	// Sovereign check (F8): OFZ federal bonds do not publish corporate 714-П material facts
	if isFederalSovereignBond(bond) {
		issuerName := bond.Issuer
		if issuerName == "" {
			issuerName = "Министерство финансов Российской Федерации"
		}
		return &FactsResult{
			INN:         sovereignINN,
			ISIN:        bond.ISIN,
			IssuerName:  issuerName,
			IsSovereign: true,
		}, nil
	}

	inn := strings.TrimSpace(bond.IssuerINN)
	if inn == "" {
		return nil, fmt.Errorf("для облигации %s (%s) не указан ИНН эмитента", bond.Name, bond.ISIN)
	}

	return s.getFactsByINN(ctx, inn, bond.ISIN, bond.Issuer, limit)
}

func (s *FactsService) getFactsByINN(ctx context.Context, inn, isin, preferredIssuerName string, limit int) (*FactsResult, error) {
	if inn == sovereignINN {
		name := preferredIssuerName
		if name == "" {
			name = "Министерство финансов Российской Федерации"
		}
		return &FactsResult{
			INN:         sovereignINN,
			ISIN:        isin,
			IssuerName:  name,
			IsSovereign: true,
		}, nil
	}

	if s.cache != nil {
		if s.cache.IsNotFound(inn) {
			return nil, fmt.Errorf("%w: ИНН %s", ErrNoMaterialFacts, inn)
		}
		if cached, ok := s.cache.Get(inn); ok {
			return buildFactsResult(inn, isin, preferredIssuerName, cached, limit), nil
		}
	}

	if s.factsProvider == nil {
		return nil, errors.New("facts provider is not configured")
	}

	facts, err := s.sf.Do(inn, func() ([]domain.MaterialFact, error) {
		if s.cache != nil {
			if s.cache.IsNotFound(inn) {
				return nil, fmt.Errorf("%w: ИНН %s", ErrNoMaterialFacts, inn)
			}
			if cached, ok := s.cache.Get(inn); ok {
				return cached, nil
			}
		}

		fetchLimit := limit
		if fetchLimit < 10 {
			fetchLimit = 10
		}

		// Detach from first caller cancellation while enforcing a bounded 10s timeout (F11.3)
		sfCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		rawFacts, fetchErr := s.factsProvider.GetRecentFacts(sfCtx, inn, fetchLimit)
		if fetchErr != nil {
			if errors.Is(fetchErr, edisclosure.ErrNoFactsFound) || errors.Is(fetchErr, ErrNoMaterialFacts) {
				if s.cache != nil {
					s.cache.SetNotFound(inn, negativeFactsCacheTTL)
				}
				return nil, fmt.Errorf("%w: ИНН %s", ErrNoMaterialFacts, inn)
			}
			return nil, fetchErr
		}

		if len(rawFacts) == 0 {
			if s.cache != nil {
				s.cache.SetNotFound(inn, negativeFactsCacheTTL)
			}
			return nil, fmt.Errorf("%w: ИНН %s", ErrNoMaterialFacts, inn)
		}

		normalized := make([]domain.MaterialFact, 0, len(rawFacts))
		for _, f := range rawFacts {
			if f.INN == "" {
				f.INN = inn
			}
			if f.Category == "" || f.Severity == "" {
				f.Classify()
			}
			normalized = append(normalized, f)
		}

		sort.Slice(normalized, func(i, j int) bool {
			return normalized[i].PublishedAt.After(normalized[j].PublishedAt)
		})

		if s.cache != nil {
			s.cache.Set(inn, normalized)
		}
		return normalized, nil
	})
	if err != nil {
		return nil, err
	}

	return buildFactsResult(inn, isin, preferredIssuerName, facts, limit), nil
}

func buildFactsResult(inn, isin, preferredIssuerName string, facts []domain.MaterialFact, limit int) *FactsResult {
	issuerName := preferredIssuerName
	for _, f := range facts {
		if issuerName == "" && f.IssuerName != "" {
			issuerName = f.IssuerName
			break
		}
	}

	sliced := facts
	if limit > 0 && len(sliced) > limit {
		sliced = sliced[:limit]
	}

	out := make([]domain.MaterialFact, len(sliced))
	copy(out, sliced)
	for i := range out {
		if out[i].ISIN == "" && isin != "" {
			out[i].ISIN = isin
		}
		if out[i].IssuerName == "" && issuerName != "" {
			out[i].IssuerName = issuerName
		}
	}

	return &FactsResult{
		INN:        inn,
		ISIN:       isin,
		IssuerName: issuerName,
		Facts:      out,
	}
}
