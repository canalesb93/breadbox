package sync

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"breadbox/internal/provider"
)

// mockBalanceProvider implements provider.Provider with controllable GetBalances behavior.
type mockBalanceProvider struct {
	provider.Provider // embed interface for methods we don't care about
	balanceCalls      int
	balanceErrors     []error // error to return on each call (index = call number)
	balanceResults    [][]provider.AccountBalance
}

func (m *mockBalanceProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	call := m.balanceCalls
	m.balanceCalls++
	if call < len(m.balanceErrors) && m.balanceErrors[call] != nil {
		return nil, m.balanceErrors[call]
	}
	if call < len(m.balanceResults) {
		return m.balanceResults[call], nil
	}
	return nil, nil
}

func TestUpdateBalancesWithRetry_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	e := &Engine{logger: logger, balanceRetryDelay: 1 * time.Millisecond}

	mock := &mockBalanceProvider{
		balanceErrors:  []error{nil},
		balanceResults: [][]provider.AccountBalance{{}},
	}

	warning := e.updateBalancesWithRetry(context.Background(), nil, mock, provider.Connection{}, logger)
	if warning != "" {
		t.Errorf("expected no warning on success, got: %s", warning)
	}
	if mock.balanceCalls != 1 {
		t.Errorf("expected 1 call, got %d", mock.balanceCalls)
	}
}

func TestUpdateBalancesWithRetry_FailThenSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	e := &Engine{logger: logger, balanceRetryDelay: 1 * time.Millisecond}

	mock := &mockBalanceProvider{
		balanceErrors:  []error{errors.New("connection timeout"), nil},
		balanceResults: [][]provider.AccountBalance{nil, {}},
	}

	warning := e.updateBalancesWithRetry(context.Background(), nil, mock, provider.Connection{}, logger)
	if warning == "" {
		t.Error("expected warning when first attempt fails but retry succeeds")
	}
	if !strings.Contains(warning, "succeeded on retry") {
		t.Errorf("warning should mention retry success, got: %s", warning)
	}
	if !strings.Contains(warning, "connection timeout") {
		t.Errorf("warning should include original error, got: %s", warning)
	}
	if mock.balanceCalls != 2 {
		t.Errorf("expected 2 calls (original + retry), got %d", mock.balanceCalls)
	}
}

func TestUpdateBalancesWithRetry_BothFail(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	e := &Engine{logger: logger, balanceRetryDelay: 1 * time.Millisecond}

	mock := &mockBalanceProvider{
		balanceErrors: []error{
			errors.New("first failure"),
			errors.New("second failure"),
		},
	}

	warning := e.updateBalancesWithRetry(context.Background(), nil, mock, provider.Connection{}, logger)
	if warning == "" {
		t.Error("expected warning when both attempts fail")
	}
	if !strings.Contains(warning, "failed after retry") {
		t.Errorf("warning should mention retry failure, got: %s", warning)
	}
	if !strings.Contains(warning, "second failure") {
		t.Errorf("warning should include retry error, got: %s", warning)
	}
	if mock.balanceCalls != 2 {
		t.Errorf("expected 2 calls, got %d", mock.balanceCalls)
	}
}
