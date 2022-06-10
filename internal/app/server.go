package app

import (
	"context"
	"crypto/rand"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth"
	"go.uber.org/zap"
	"net/http"
)

func RunServerApp(ctx context.Context, serverAddress string, logger *zap.Logger, st StorageServices) {
	privateKey := make([]byte, 32)
	readBytes, err := rand.Read(privateKey)
	if err != nil || readBytes != len(privateKey) {
		logger.Fatal("Failed to generate private key", zap.Error(err), zap.Int("generated_len", readBytes))
	}

	authorizer := jwtauth.New("HS256", privateKey, nil)

	authServer, err := NewAuthServer(ctx, logger, st.UserStorage, authorizer)
	if err != nil {
		logger.Fatal("Failed to initialize auth server", zap.Error(err))
	}

	martServer, err := NewAppServer(ctx, logger, st, authorizer)
	if err != nil {
		logger.Fatal("Failed to initialize app server", zap.Error(err))
	}

	r := chi.NewRouter()
	r.Use(middleware.NoCache)
	r.Use(middleware.Compress(CompressionLevel))
	r.Use(DecompressGzip)

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

		r.Post("/api/user/orders", martServer.apiAddUserOrder)
	})

	server := &http.Server{Addr: serverAddress, Handler: r}
	server.ListenAndServe()
}
