package storage

import (
	"context"
	"errors"
	"time"
)

var ErrNotEnoughBalance = errors.New("not enough balance")

type BalanceInfo struct {
	Current   float64 `json:"current"`
	Withdrawn int64   `json:"withdrawn"`
}

type Withdrawal struct {
	Order       int64
	Sum         int64
	ProcessedAt time.Time
}

type WithdrawalStorage interface {
	Withdraw(ctx context.Context, userID, order, sum int64) error
	GetBalance(ctx context.Context, userID int64) (*BalanceInfo, error)
	GetWithdrawals(ctx context.Context, userID int64) ([]Withdrawal, error)
}
