package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Lutefd/challenge-bravo/internal/cache"
	"github.com/Lutefd/challenge-bravo/internal/model"
	"github.com/Lutefd/challenge-bravo/internal/repository"
	"github.com/Lutefd/challenge-bravo/internal/worker"
	"github.com/google/uuid"
)

type CurrencyService struct {
	repo        repository.CurrencyRepository
	cache       cache.Cache
	externalAPI worker.ExternalAPIClient
}

func NewCurrencyService(repo repository.CurrencyRepository, cache cache.Cache, externalAPI worker.ExternalAPIClient) *CurrencyService {
	return &CurrencyService{
		repo:        repo,
		cache:       cache,
		externalAPI: externalAPI,
	}
}

func (s *CurrencyService) Convert(ctx context.Context, from, to string, amount float64) (float64, error) {
	fromRate, err := s.getRate(ctx, from)
	if err != nil {
		return 0, err
	}

	toRate, err := s.getRate(ctx, to)
	if err != nil {
		return 0, err
	}
	usdAmount := amount / fromRate
	result := usdAmount * toRate

	return result, nil
}

func (s *CurrencyService) getRate(ctx context.Context, code string) (float64, error) {
	rate, err := s.cache.Get(ctx, code)
	if err == nil {
		return rate, nil
	}
	currency, err := s.repo.GetByCode(ctx, code)
	if err == nil {
		s.cache.Set(ctx, code, currency.Rate, 1*time.Hour)
		return currency.Rate, nil
	}
	rates, err := s.externalAPI.FetchRates(ctx)
	if err != nil {
		return 0, err
	}

	rate, ok := rates.Rates[code]
	if !ok {
		return 0, fmt.Errorf("currency %s not found", code)
	}
	currency = &model.Currency{
		Code:      strings.ToUpper(code),
		Rate:      rate,
		UpdatedAt: time.Now(),
	}
	err = s.repo.Create(ctx, currency)
	if err != nil {
		return 0, err
	}

	s.cache.Set(ctx, code, rate, 1*time.Hour)

	return rate, nil
}

func (s *CurrencyService) AddCurrency(ctx context.Context, currency *model.Currency) error {
	_, err := s.repo.GetByCode(ctx, currency.Code)
	if err == nil {
		return fmt.Errorf("currency %s already exists", currency.Code)
	}

	if err := s.repo.Create(ctx, currency); err != nil {
		return fmt.Errorf("failed to add currency to repository: %w", err)
	}

	if err := s.cache.Set(ctx, currency.Code, currency.Rate, 1*time.Hour); err != nil {
		fmt.Printf("failed to update cache for new currency %s: %v\n", currency.Code, err)
	}

	return nil
}

func (s *CurrencyService) UpdateCurrency(ctx context.Context, code string, rate float64, updatedBy uuid.UUID) error {
	currency, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		if err == model.ErrCurrencyNotFound {
			return model.ErrCurrencyNotFound
		}
		return fmt.Errorf("failed to get currency: %w", err)
	}

	currency.Rate = rate
	currency.UpdatedAt = time.Now()
	currency.UpdatedBy = updatedBy

	if err := s.repo.Update(ctx, currency); err != nil {
		return fmt.Errorf("failed to update currency in repository: %w", err)
	}

	if err := s.cache.Set(ctx, code, rate, 1*time.Hour); err != nil {
		fmt.Printf("failed to update cache for currency %s: %v\n", code, err)
	}

	return nil
}

func (s *CurrencyService) RemoveCurrency(ctx context.Context, code string) error {
	_, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return fmt.Errorf("currency %s not found", code)
	}

	if err := s.repo.Delete(ctx, code); err != nil {
		return fmt.Errorf("failed to remove currency from repository: %w", err)
	}

	if err := s.cache.Delete(ctx, code); err != nil {
		fmt.Printf("failed to remove currency %s from cache: %v\n", code, err)
	}
	return nil
}
