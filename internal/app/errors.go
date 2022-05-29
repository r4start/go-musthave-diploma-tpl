package app

import "errors"

var (
	ErrBadContentType  = errors.New("bad content type in request")
	ErrBodyUnmarshal   = errors.New("failed to unmarshal request body")
	ErrMissedJWTKey    = errors.New("failed to get data from JWT")
	ErrJWTKeyBadFormat = errors.New("JWT key data has unexpected type")
)
