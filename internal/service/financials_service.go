package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
	"telegram-bot-moex/internal/store"
)

var (
	// Security Whitelists for user input
	innRegex    = regexp.MustCompile(`^[0-9]{10}$|^[0-9]{12}$`)
	isinRegex   = regexp.MustCompile(`^[A-Z0-9]{12}$`)
	tickerRegex = regexp.MustCompile(`^[A-Z0-9.\-_]{2,20}$`)

	// Domain/Service sentinel errors
	ErrInvalidIdentifier  = errors.New("неверный формат запроса: укажите ISIN (12 знаков), ИНН (10 или 12 цифр) или тикер")
	ErrInvalidINNChecksum = errors.New("неверная контрольная сумма ИНН")
	ErrIssuerNotFound     = errors.New("эмитент не найден в ГИР БО")
	ErrBondNotFound       = errors.New("облигация не найдена")

	// Sovereign INN for Ministry of Finance (OFZ bonds)
	sovereignINN = "7710168360"

	negativeCacheTTL = 2 * time.Hour
)

type sfCall struct {
	wg  sync.WaitGroup
	val *domain.IssuerBalance
	err error
}

type singleflightGroup struct {
	mu sync.Mutex
	m  map[string]*sfCall
}

func (g *singleflightGroup) Do(key string, fn func() (*domain.IssuerBalance, error)) (*domain.IssuerBalance, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*sfCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(sfCall)
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

// FinancialsService coordinates issuer balance sheet lookups, validation, and caching.
type FinancialsService struct {
	bondProvider provider.BondProvider
	bfoProvider  provider.FinancialsProvider
	cache        store.FinancialsCache
	sf           singleflightGroup
}

// NewFinancialsService creates a new FinancialsService.
func NewFinancialsService(bp provider.BondProvider, bfo provider.FinancialsProvider, cache store.FinancialsCache) *FinancialsService {
	return &FinancialsService{
		bondProvider: bp,
		bfoProvider:  bfo,
		cache:        cache,
	}
}

// GetIssuerBalance retrieves balance sheet by ISIN, Ticker, or directly by INN.
func (s *FinancialsService) GetIssuerBalance(ctx context.Context, identifier string) (*domain.IssuerBalance, error) {
	id := strings.TrimSpace(strings.ToUpper(identifier))
	if id == "" {
		return nil, ErrInvalidIdentifier
	}

	// 1. Security Check: Strict input format whitelist
	isINN := innRegex.MatchString(id)
	isISIN := isinRegex.MatchString(id)
	isTicker := tickerRegex.MatchString(id)

	if !isINN && !isISIN && !isTicker {
		return nil, ErrInvalidIdentifier
	}

	// 2. Direct INN query
	if isINN {
		if !domain.IsValidINN(id) {
			return nil, ErrInvalidINNChecksum
		}
		return s.getBalanceByINN(ctx, id, "")
	}

	// 3. Resolve bond first to obtain its IssuerINN
	bond, err := s.bondProvider.GetBond(ctx, id)
	if err != nil {
		// Try search fallback if lookupKey was a ticker or partial query
		results, searchErr := s.bondProvider.SearchBonds(ctx, id)
		if searchErr != nil || len(results) == 0 {
			return nil, fmt.Errorf("%w: %s", ErrBondNotFound, identifier)
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
				return nil, fmt.Errorf("%w: неоднозначный тикер %s", ErrBondNotFound, identifier)
			}
		}
		bond, err = s.bondProvider.GetBond(ctx, matchedISIN)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrBondNotFound, identifier)
		}
	}

	// F4: Sovereign detection - require exact sovereign federal INN or exact federal Ministry name.
	// Regional/sub-federal finance ministries (e.g. "Минфин Нижегородской области") do NOT qualify as federal sovereign debt.
	isFederalSovereign := bond.IssuerINN == sovereignINN ||
		strings.EqualFold(strings.TrimSpace(bond.Issuer), "Министерство финансов Российской Федерации") ||
		strings.EqualFold(strings.TrimSpace(bond.Issuer), "Минфин РФ")

	if isFederalSovereign {
		return &domain.IssuerBalance{
			INN:         sovereignINN,
			IssuerName:  bond.Issuer,
			IsSovereign: true,
			RiskLevel:   domain.RiskSovereign,
			Source:      "Минфин РФ / ОФЗ",
			FetchedAt:   time.Now(),
		}, nil
	}

	if bond.IssuerINN == "" {
		return nil, fmt.Errorf("для облигации %s (%s) не указан ИНН эмитента", bond.Name, bond.ISIN)
	}

	return s.getBalanceByINN(ctx, bond.IssuerINN, bond.Issuer)
}

func (s *FinancialsService) getBalanceByINN(ctx context.Context, inn string, preferredName string) (*domain.IssuerBalance, error) {
	// Sovereign check
	if inn == sovereignINN {
		name := "Министерство финансов Российской Федерации"
		if preferredName != "" {
			name = preferredName
		}
		return &domain.IssuerBalance{
			INN:         sovereignINN,
			IssuerName:  name,
			IsSovereign: true,
			RiskLevel:   domain.RiskSovereign,
			Source:      "Минфин РФ / ОФЗ",
			FetchedAt:   time.Now(),
		}, nil
	}

	// Check cache (positive and negative)
	if s.cache != nil {
		if s.cache.IsNotFound(inn) {
			return nil, fmt.Errorf("%w: ИНН %s", ErrIssuerNotFound, inn)
		}
		if cached, ok := s.cache.Get(inn); ok {
			return cached, nil
		}
	}

	// Singleflight deduplication: prevent dog-pile queries toward upstream FNS API
	balance, err := s.sf.Do(inn, func() (*domain.IssuerBalance, error) {
		// Double-check cache in case another flight populated it
		if s.cache != nil {
			if s.cache.IsNotFound(inn) {
				return nil, fmt.Errorf("%w: ИНН %s", ErrIssuerNotFound, inn)
			}
			if cached, ok := s.cache.Get(inn); ok {
				return cached, nil
			}
		}

		b, fetchErr := s.bfoProvider.GetIssuerBalance(ctx, inn)
		if fetchErr != nil {
			errStr := strings.ToLower(fetchErr.Error())
			// Negative cache on not found / no reports / empty statements
			if strings.Contains(errStr, "not found") || strings.Contains(errStr, "no reports") || strings.Contains(errStr, "no bfo reports") || strings.Contains(errStr, "no financial statements") || strings.Contains(errStr, "no valid balance") {
				if s.cache != nil {
					s.cache.SetNotFound(inn, negativeCacheTTL)
				}
				return nil, fmt.Errorf("%w: ИНН %s (%v)", ErrIssuerNotFound, inn, fetchErr)
			}
			return nil, fetchErr
		}

		if preferredName != "" && b.IssuerName == "" {
			b.IssuerName = preferredName
		}

		if s.cache != nil {
			s.cache.Set(inn, b)
		}
		return b, nil
	})

	if err != nil {
		return nil, err
	}

	return balance, nil
}
