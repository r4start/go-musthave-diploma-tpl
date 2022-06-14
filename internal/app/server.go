package app

import (
	"context"
	"crypto/rand"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth"
	"github.com/r4start/go-musthave-diploma-tpl/internal/storage"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func RunServerApp(ctx context.Context, serverAddress string, logger *zap.Logger, st storage.AppStorage) {
	privateKey := make([]byte, 32)
	readBytes, err := rand.Read(privateKey)
	if err != nil || readBytes != len(privateKey) {
		logger.Fatal("Failed to generate private key", zap.Error(err), zap.Int("generated_len", readBytes))
	}

	authorizer := jwtauth.New("HS256", privateKey, nil)

	authServer, err := NewAuthServer(ctx, logger, st, authorizer)
	if err != nil {
		logger.Fatal("Failed to initialize auth server", zap.Error(err))
	}

	martServer, err := NewAppServer(ctx, logger, st)
	if err != nil {
		logger.Fatal("Failed to initialize app server", zap.Error(err))
	}

	r := chi.NewRouter()
	r.Use(middleware.NoCache)
	r.Use(middleware.Compress(CompressionLevel))
	r.Use(DecompressGzip)
	r.Use(middleware.Timeout(60 * time.Second))

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})

	r.Group(func(r chi.Router) {
		r.Post("/api/user/register", authServer.apiUserRegister)
		r.Post("/api/user/login", authServer.apiUserLogin)
	})

	r.Group(func(r chi.Router) {
		r.Use(jwtauth.Verifier(authorizer))
		r.Use(jwtauth.Authenticator)
		r.Use(AppAuthorization(st))

		r.Post("/api/user/orders", martServer.apiAddUserOrder)
		r.Get("/api/user/orders", martServer.apiGetUserOrders)

		r.Get("/api/user/balance", martServer.apiGetUserBalance)
		r.Get("/api/user/balance/withdrawals", martServer.apiGetUserWithdrawals)
		r.Post("/api/user/balance/withdraw", martServer.apiBalanceWithdraw)
	})

	server := &http.Server{Addr: serverAddress, Handler: r}
	server.ListenAndServe()
}
