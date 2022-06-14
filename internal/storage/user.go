package storage

import (
	"context"
	"errors"
)

const (
	UserStateActive   = "active"
	UserStateDisabled = "disabled"
)

var (
	ErrDuplicateUser = errors.New("duplicate user")
	ErrNoSuchUser    = errors.New("no such user")
)

type UserAuthorization struct {
	ID       int64
	UserName string
	Secret   []byte
	State    string
}

type UserStorage interface {
	AddUser(ctx context.Context, auth *UserAuthorization) error
	GetUserAuthInfo(ctx context.Context, userName string) (*UserAuthorization, error)
	GetUserAuthInfoByID(ctx context.Context, userID int64) (*UserAuthorization, error)
}
