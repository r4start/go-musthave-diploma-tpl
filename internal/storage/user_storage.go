package storage

import "errors"

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
	Add(auth *UserAuthorization) error
	Get(userName string) (*UserAuthorization, error)
	GetByID(userID int64) (*UserAuthorization, error)
}
