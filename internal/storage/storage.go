package storage

import (
	"context"
	"errors"
	"time"
)

const (
	UserStateActive   = "active"
	UserStateDisabled = "disabled"

	StatusNew        = "NEW"
	StatusInvalid    = "INVALID"
	StatusProcessing = "PROCESSING"
	StatusProcessed  = "PROCESSED"
)

var (
	ErrDuplicateUser      = errors.New("duplicate user")
	ErrNoSuchUser         = errors.New("no such user")
	ErrNotEnoughBalance   = errors.New("not enough balance")
	ErrDuplicateOrder     = errors.New("duplicate order")
	ErrOrderAlreadyPlaced = errors.New("order already placed")
)

type UserAuthorization struct {
	ID       int64
	UserName string
	Secret   []byte
	State    string
}

type BalanceInfo struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type Withdrawal struct {
	Order       int64
	Sum         float64
	ProcessedAt time.Time
}

type Order struct {
	ID         int64
	UserID     int64
	Status     string
	Accrual    float64
	UploadedAt time.Time
}

type AppStorage interface {
	AddUser(ctx context.Context, auth *UserAuthorization) error
	GetUserAuthInfo(ctx context.Context, userName string) (*UserAuthorization, error)
	GetUserAuthInfoByID(ctx context.Context, userID int64) (*UserAuthorization, error)

	Withdraw(ctx context.Context, userID, order int64, sum float64) error
	AddBalance(ctx context.Context, userID int64, amount float64) error
	UpdateBalanceFromOrders(ctx context.Context, orders []Order) error
	GetBalance(ctx context.Context, userID int64) (*BalanceInfo, error)
	GetWithdrawals(ctx context.Context, userID int64) ([]Withdrawal, error)

	AddOrder(ctx context.Context, userID, orderID int64) error
	UpdateOrder(ctx context.Context, order Order) error
	GetOrders(ctx context.Context, userID int64) ([]Order, error)
	GetUnfinishedOrders(ctx context.Context) ([]Order, error)
}
