package app

import (
	"compress/gzip"
	"context"
	"github.com/go-chi/jwtauth"
	"github.com/r4start/go-musthave-diploma-tpl/internal/storage"
	"net/http"
)

var UserAuthDataCtxKey = &contextKey{"UserAuthData"}

type gzipBodyReader struct {
	gzipReader *gzip.Reader
}

func (gz *gzipBodyReader) Read(p []byte) (n int, err error) {
	return gz.gzipReader.Read(p)
}

func (gz *gzipBodyReader) Close() error {
	return gz.gzipReader.Close()
}

func DecompressGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
			r.Body = &gzipBodyReader{gzipReader: gz}
		}
		next.ServeHTTP(w, r)
	})
}

func AppAuthorization(st storage.AppStorage) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		authFn := func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			_, claims, err := jwtauth.FromContext(ctx)
			if err != nil {
				http.Error(w, "", http.StatusUnauthorized)
				return
			}
			userID := int64(0)
			if id, exists := claims["id"]; exists {
				switch value := id.(type) {
				case int:
					userID = int64(value)
				case int64:
					userID = value
				case float64:
					userID = int64(value)
				default:
					http.Error(w, "", http.StatusUnauthorized)
					return
				}
			}

			userData, err := st.GetUserAuthInfoByID(ctx, userID)
			if err != nil {
				http.Error(w, "", http.StatusUnauthorized)
				return
			}

			if userData.State != storage.UserStateActive {
				http.Error(w, "", http.StatusUnauthorized)
				return
			}

			ctx = context.WithValue(ctx, UserAuthDataCtxKey, userData)

			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(authFn)
	}
}

type contextKey struct {
	name string
}

func (k *contextKey) String() string {
	return "marketappauth context value " + k.name
}
