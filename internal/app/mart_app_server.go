package app

import (
	"context"
	"encoding/json"
	"github.com/go-chi/jwtauth"
	"github.com/r4start/go-musthave-diploma-tpl/internal/storage"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strconv"
)

type MartServer struct {
	ctx         context.Context
	logger      *zap.Logger
	userStorage storage.UserStorage
	authorizer  *jwtauth.JWTAuth
}

func NewAppServer(ctx context.Context, logger *zap.Logger, userStorage storage.UserStorage, authorizer *jwtauth.JWTAuth) (*MartServer, error) {
	server := &MartServer{
		ctx:         ctx,
		logger:      logger,
		userStorage: userStorage,
		authorizer:  authorizer,
	}

	return server, nil
}

func (s *MartServer) apiUserOrders(w http.ResponseWriter, r *http.Request) {
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

	userID, err := s.getUserID(r)
	if err != nil {
		s.logger.Error("failed to get user id", zap.Error(err))
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	userData, err := s.userStorage.GetByID(userID)
	if err != nil {
		s.logger.Error("failed to get user id", zap.Error(err))
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	if userData.State != storage.UserStateActive {
		s.logger.Error("request from disabled user", zap.Error(err))
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	orderID, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		s.logger.Error("failed to get order id", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if !IsValidLuhn(orderID) {
		s.logger.Error("bad order id", zap.Int64("order_id", orderID))
		http.Error(w, "", http.StatusUnprocessableEntity)
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

func (s *MartServer) getUserID(r *http.Request) (int64, error) {
	jwtCookie, err := r.Cookie(AuthCookie)
	if err != nil {
		return 0, err
	}

	token, err := s.authorizer.Decode(jwtCookie.Value)
	if err != nil {
		return 0, err
	}

	if v, exists := token.Get("id"); exists {
		switch v.(type) {
		case int:
			return int64(v.(int)), nil
		case int64:
			return v.(int64), nil
		case float64:
			return int64(v.(float64)), nil
		default:
			return 0, ErrJWTKeyBadFormat
		}
	}

	return 0, ErrMissedJWTKey
}
