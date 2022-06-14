package accrual

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/r4start/go-musthave-diploma-tpl/internal/app"
	"github.com/r4start/go-musthave-diploma-tpl/internal/storage"
	"go.uber.org/zap"
	"net/http"
	"time"
)

const (
	StatusRegistered = "REGISTERED"
	StatusInvalid    = "INVALID"
	StatusProcessing = "PROCESSING"
	StatusProcessed  = "PROCESSED"
)

type orderInfo struct {
	Order   string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float64 `json:"accrual"`
}

type Updater struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	client    *resty.Client
	baseAddr  string
	storage   app.StorageServices
	logger    *zap.Logger
}

func NewUpdater(ctx context.Context, baseAddr string, updateRps int, storage app.StorageServices, logger *zap.Logger) *Updater {
	ctx, cancel := context.WithCancel(ctx)

	client := resty.New()

	updater := &Updater{
		ctx:       ctx,
		ctxCancel: cancel,
		client:    client,
		baseAddr:  baseAddr,
		storage:   storage,
		logger:    logger,
	}

	go updater.updateOrders()

	return updater
}

func (u *Updater) Stop() {
	u.ctxCancel()
}

func (u *Updater) updateOrders() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			u.update()
		case <-u.ctx.Done():
			return

		}
	}
}

func (u *Updater) update() {
	orders, err := u.storage.GetUnfinishedOrders(u.ctx)
	if err != nil {
		u.logger.Error("failed to get unfinished orders", zap.Error(err))
		return
	}
	if len(orders) == 0 {
		u.logger.Info("no orders to update")
		return
	}

	for _, o := range orders {
		info, err := u.getOrderStatus(o.ID)
		if err != nil {
			u.logger.Error("failed to get order info", zap.Int64("order_id", o.ID), zap.Error(err))
			continue
		}

		switch info.Status {
		case StatusRegistered:
			o.Status = storage.StatusProcessing
		case StatusInvalid:
			o.Status = storage.StatusInvalid
		case StatusProcessing:
			o.Status = storage.StatusProcessing
		case StatusProcessed:
			o.Status = storage.StatusProcessed
			o.Accrual = info.Accrual
		}

		if err := u.storage.UpdateOrder(u.ctx, o); err != nil {
			u.logger.Error("failed to update order", zap.Int64("order_id", o.ID), zap.Error(err))
			continue
		}

		if info.Status == storage.StatusProcessed {
			if err := u.storage.AddBalance(u.ctx, o.UserID, o.Accrual); err != nil {
				u.logger.Error("failed to update user balance", zap.Int64("user_id", o.UserID), zap.Error(err))
			}
		}
	}
}

func (u *Updater) getOrderStatus(orderID int64) (*orderInfo, error) {
	request := u.client.R().SetContext(u.ctx)

	url := fmt.Sprintf("%s/api/orders/%d", u.baseAddr, orderID)
	response, err := request.Get(url)
	if err != nil {
		return nil, err
	}

	if response.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", response.StatusCode())
	}

	var info orderInfo
	if err := json.Unmarshal(response.Body(), &info); err != nil {
		return nil, err
	}

	return &info, nil
}
