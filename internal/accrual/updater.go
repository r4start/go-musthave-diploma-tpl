package accrual

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/r4start/go-musthave-diploma-tpl/internal/storage"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"sync"
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

type Config struct {
	BaseAddr string
	Logger   *zap.Logger
	storage.AppStorage
}

type Updater struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	client    *resty.Client
	Config
}

func NewUpdater(ctx context.Context, cfg Config) *Updater {
	ctx, cancel := context.WithCancel(ctx)

	retryFunc := resty.RetryAfterFunc(func(client *resty.Client, response *resty.Response) (time.Duration, error) {
		if response.StatusCode() != http.StatusTooManyRequests {
			return 0, nil
		}

		retryAfterValue := response.Header().Get("Retry-After")
		if len(retryAfterValue) == 0 {
			return 0, nil
		}

		seconds, err := strconv.ParseInt(retryAfterValue, 10, 64)
		if err != nil {
			return 0, err
		}

		return time.Duration(seconds) * time.Second, nil
	})

	client := resty.New().SetRetryAfter(retryFunc).SetRetryCount(3)

	updater := &Updater{
		ctx:       ctx,
		ctxCancel: cancel,
		client:    client,
		Config:    cfg,
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
	orders, err := u.GetUnfinishedOrders(u.ctx)
	if err != nil {
		u.Logger.Error("failed to get unfinished orders", zap.Error(err))
		return
	}
	if len(orders) == 0 {
		u.Logger.Info("no orders to update")
		return
	}

	var wg sync.WaitGroup
	ordersInfo := make([]*orderInfo, len(orders))

	ordersWithBalanceUpdate := make([]storage.Order, 0)
	for i, o := range orders {
		wg.Add(1)
		go func(index int, o storage.Order) {
			defer wg.Done()
			info, err := u.getOrderStatus(o.ID)
			if err != nil {
				u.Logger.Error("failed to get order info", zap.Int64("order_id", o.ID), zap.Error(err))
				return
			}
			ordersInfo[index] = info
		}(i, o)
	}

	wg.Wait()

	for i, info := range ordersInfo {
		if info == nil {
			continue
		}

		switch info.Status {
		case StatusRegistered:
			orders[i].Status = storage.StatusProcessing
		case StatusInvalid:
			orders[i].Status = storage.StatusInvalid
		case StatusProcessing:
			orders[i].Status = storage.StatusProcessing
		case StatusProcessed:
			orders[i].Status = storage.StatusProcessed
			orders[i].Accrual = info.Accrual
		}

		if info.Status == storage.StatusProcessed {
			ordersWithBalanceUpdate = append(ordersWithBalanceUpdate, orders[i])
			continue
		}

		if err := u.UpdateOrder(u.ctx, orders[i]); err != nil {
			u.Logger.Error("failed to update order", zap.Int64("order_id", orders[i].ID), zap.Error(err))
		}
	}

	if err := u.UpdateBalanceFromOrders(u.ctx, ordersWithBalanceUpdate); err != nil {
		u.Logger.Error("failed to update user balance", zap.Error(err))
	}
}

func (u *Updater) getOrderStatus(orderID int64) (*orderInfo, error) {
	request := u.client.R().SetContext(u.ctx)

	url := fmt.Sprintf("%s/api/orders/%d", u.BaseAddr, orderID)
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
