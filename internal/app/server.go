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

const (
	privateKeySize           = 32
	compressionLevel         = 7
	requestProcessingTimeout = 60 * time.Second
)

func RunServerApp(ctx context.Context, serverAddress string, logger *zap.Logger, st storage.AppStorage) {
	privateKey := make([]byte, privateKeySize)
	readBytes, err := rand.Read(privateKey)
	if err != nil || readBytes != privateKeySize {
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
	r.Use(middleware.Compress(compressionLevel))
	r.Use(DecompressGzip)
	r.Use(middleware.Timeout(requestProcessingTimeout))

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
		r.Use(AuthorizationVerifier(st))

		r.Route("/api/user/orders", func(r chi.Router) {
			r.Get("/", martServer.apiGetUserOrders)
			r.Post("/", martServer.apiAddUserOrder)
		})

		r.Route("/api/user/balance", func(r chi.Router) {
			r.Get("/", martServer.apiGetUserBalance)
			r.Get("/withdrawals", martServer.apiGetUserWithdrawals)
			r.Post("/withdraw", martServer.apiBalanceWithdraw)
		})
	})

	server := &http.Server{Addr: serverAddress, Handler: r}
	server.ListenAndServe()
}
