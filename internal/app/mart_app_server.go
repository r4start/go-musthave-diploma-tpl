package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/r4start/go-musthave-diploma-tpl/internal/storage"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strconv"
	"time"
)

type MartServer struct {
	ctx            context.Context
	logger         *zap.Logger
	storageService storage.AppStorage
}

func NewAppServer(ctx context.Context, logger *zap.Logger, storage storage.AppStorage) (*MartServer, error) {
	server := &MartServer{
		ctx:            ctx,
		logger:         logger,
		storageService: storage,
	}

	return server, nil
}

func (s *MartServer) apiAddUserOrder(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != "text/plain" {
		s.logger.Error("bad content type", zap.String("content_type", contentType))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if !IsValidLuhn(string(b)) {
		s.logger.Error("bad order id", zap.String("order_id", string(b)))
		http.Error(w, "", http.StatusUnprocessableEntity)
		return
	}

	orderID, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		s.logger.Error("failed to get order id", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	if err := s.storageService.AddOrder(r.Context(), userData.ID, orderID); err != nil {
		if errors.Is(err, storage.ErrDuplicateOrder) {
			s.logger.Error("duplicate order id", zap.Int64("order_id", orderID))
			http.Error(w, "", http.StatusConflict)
			return
		}
		if errors.Is(err, storage.ErrOrderAlreadyPlaced) {
			s.logger.Info("order already placed", zap.Int64("order_id", orderID))
			w.WriteHeader(http.StatusOK)
			return
		}
		s.logger.Error("failed to add order", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *MartServer) apiGetUserOrders(w http.ResponseWriter, r *http.Request) {
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	orders, err := s.storageService.GetOrders(r.Context(), userData.ID)
	if err != nil {
		s.logger.Error("get orders failed", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	respData := make([]orderResponse, len(orders))
	for i, e := range orders {
		respData[i] = orderResponse{
			Number:     strconv.FormatInt(e.ID, 10),
			Status:     e.Status,
			Accrual:    e.Accrual,
			UploadedAt: e.UploadedAt,
		}
	}

	s.apiWriteResponse(w, http.StatusOK, respData)
}

func (s *MartServer) apiGetUserWithdrawals(w http.ResponseWriter, r *http.Request) {
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	ws, err := s.storageService.GetWithdrawals(r.Context(), userData.ID)
	if err != nil {
		s.logger.Error("failed to get withdrawals", zap.Int64("user_id", userData.ID), zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if len(ws) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	responseData := make([]withdrawalsResponse, len(ws))
	for i, e := range ws {
		responseData[i] = withdrawalsResponse{
			Order:       strconv.FormatInt(e.Order, 10),
			Sum:         e.Sum,
			ProcessedAt: e.ProcessedAt,
		}
	}

	s.apiWriteResponse(w, http.StatusOK, responseData)
}

func (s *MartServer) apiGetUserBalance(w http.ResponseWriter, r *http.Request) {
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	balance, err := s.storageService.GetBalance(r.Context(), userData.ID)
	if err != nil {
		s.logger.Error("failed to get balance", zap.Int64("user_id", userData.ID), zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	s.apiWriteResponse(w, http.StatusOK, balance)
}

func (s *MartServer) apiBalanceWithdraw(w http.ResponseWriter, r *http.Request) {
	userData := r.Context().Value(UserAuthDataCtxKey).(*storage.UserAuthorization)

	withdrawRequest := balanceWithdrawRequest{}
	if err := s.apiParseRequest(r, &withdrawRequest); err != nil {
		s.logger.Error("failed to withdraw balance", zap.Int64("user_id", userData.ID), zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if !IsValidLuhn(withdrawRequest.Order) {
		s.logger.Error("bad order id", zap.String("order_id", withdrawRequest.Order))
		http.Error(w, "", http.StatusUnprocessableEntity)
		return
	}

	orderID, err := strconv.ParseInt(withdrawRequest.Order, 10, 64)
	if err != nil {
		s.logger.Error("bad order id", zap.String("order_id", withdrawRequest.Order))
		http.Error(w, "", http.StatusUnprocessableEntity)
		return
	}

	err = s.storageService.Withdraw(r.Context(), userData.ID, orderID, withdrawRequest.Sum)
	if err != nil {
		if err == storage.ErrNotEnoughBalance {
			http.Error(w, "", http.StatusPaymentRequired)
			return
		}
		s.logger.Error("failed to withdraw", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *MartServer) apiParseRequest(r *http.Request, body interface{}) error {
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		s.logger.Error("bad content type", zap.String("content_type", contentType))
		return ErrBadContentType
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		return err
	}

	if err = json.Unmarshal(b, &body); err != nil {
		s.logger.Error("failed to unmarshal request json", zap.Error(err))
		return ErrBodyUnmarshal
	}

	return nil
}

func (s *MartServer) apiWriteResponse(w http.ResponseWriter, statusCode int, response interface{}) {
	dst, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("failed to marshal response", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if _, err := w.Write(dst); err != nil {
		s.logger.Error("failed to write response body", zap.Error(err))
	}
}

type orderResponse struct {
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    float64   `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type withdrawalsResponse struct {
	Order       string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

type balanceWithdrawRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}
