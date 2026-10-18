package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
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

// FinancialsService coordinates issuer balance sheet lookups, validation, and caching.
type FinancialsService struct {
	bondProvider provider.BondProvider
	bfoProvider  provider.FinancialsProvider
	cache        store.FinancialsCache
	sf           singleflightGroup[*domain.IssuerBalance]
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
	bond, err := resolveBondByIdentifier(ctx, s.bondProvider, id, identifier)
	if err != nil {
		return nil, err
	}

	if isFederalSovereignBond(bond) {
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
		if s.cache != nil {
			if s.cache.IsNotFound(inn) {
				return nil, fmt.Errorf("%w: ИНН %s", ErrIssuerNotFound, inn)
			}
			if cached, ok := s.cache.Get(inn); ok {
				return cached, nil
			}
		}

		sfCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		b, fetchErr := s.bfoProvider.GetIssuerBalance(sfCtx, inn)
		if fetchErr != nil {
			errStr := strings.ToLower(fetchErr.Error())
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
