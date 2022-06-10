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

type OrderStatus int

type Order struct {
	ID         int64       `json:"number"`
	Status     OrderStatus `json:"status"`
	Accrual    int64       `json:"accrual"`
	UploadedAt time.Time   `json:"uploaded_at"`
}

type OrderStorage interface {
	AddOrder(ctx context.Context, userID, orderID int64) error
	GetOrders(ctx context.Context, userID int64) ([]Order, error)
}
