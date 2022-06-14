package storage

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDuplicateOrder     = errors.New("duplicate order")
	ErrOrderAlreadyPlaced = errors.New("order already placed")
)

const (
	StatusRegistered = "REGISTERED"
	StatusInvalid    = "INVALID"
	StatusProcessing = "PROCESSING"
	StatusProcessed  = "PROCESSED"
)

type Order struct {
	ID         int64
	Status     string
	Accrual    int64
	UploadedAt time.Time
}

type OrderStorage interface {
	AddOrder(ctx context.Context, userID, orderID int64) error
	UpdateOrder(ctx context.Context, order Order) error
	GetOrders(ctx context.Context, userID int64) ([]Order, error)
	GetUnfinishedOrders(ctx context.Context) ([]Order, error)
}
