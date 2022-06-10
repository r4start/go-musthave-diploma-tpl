package storage

import (
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
	AddUser(auth *UserAuthorization) error
	GetUserAuthInfo(userName string) (*UserAuthorization, error)
	GetUserAuthInfoByID(userID int64) (*UserAuthorization, error)
}
