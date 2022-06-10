package storage

import (
	"context"
	"errors"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
	"time"
)

const (
	CreateStateEnum = `create type state as enum ('active', 'disabled');`

	CreateUsersTableScheme = `
       create table users (
			id bigserial PRIMARY KEY,
			name varchar(8192) not null UNIQUE,
			secret bytea not null,
			added timestamptz not null DEFAULT now(),
			flags state not null DEFAULT 'active'
		);`

	CreateUserNameIndex = `create index username_idx on users(name);`

	CheckUsersTable = `select count(*) from users;`

	AddUserQuery = `insert into users (name, secret) values ($1, $2);`

	GetUserQuery     = `select id, name, secret from users where name = $1 and flags = 'active';`
	GetUserByIDQuery = `select name, secret from users where id = $1 and flags = 'active';`

	CreateOrdersTableScheme = `
       create table orders (
			id bigserial primary key,
			number bigint not null unique,
			user_id bigint not null,
			updated_at timestamptz not null default now(),

			FOREIGN KEY (user_id)
      			REFERENCES users(id)
		);`

	CheckOrdersTable = `select count(*) from orders;`

	AddOrder     = `insert into orders (number, user_id) values ($1, $2);`
	GetOrderUser = `select user_id from orders where number = $1;`

	DatabaseOperationTimeout = 500000 * time.Second

	UniqueViolationCode = "23505"
)

type pgxStorage struct {
	ctx    context.Context
	dbConn *pgxpool.Pool
}

func NewDatabaseStorage(ctx context.Context, connection *pgxpool.Pool) (UserStorage, OrderStorage, error) {
	if err := connection.Ping(ctx); err != nil {
		return nil, nil, err
	}

	if err := prepareUsersTable(ctx, connection); err != nil {
		return nil, nil, err
	}

	if err := prepareOrdersTable(ctx, connection); err != nil {
		return nil, nil, err
	}

	storage := &pgxStorage{
		ctx:    ctx,
		dbConn: connection,
	}
	return storage, storage, nil
}

func (p *pgxStorage) AddUser(auth *UserAuthorization) error {
	opCtx, cancel := context.WithTimeout(p.ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	_, err = tx.Exec(opCtx, AddUserQuery, auth.UserName, auth.Secret)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == UniqueViolationCode {
				return ErrDuplicateUser
			}
		}
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) GetUserAuthInfo(userName string) (*UserAuthorization, error) {
	opCtx, cancel := context.WithTimeout(p.ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, GetUserQuery, userName)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	if r.Next() {
		authData := UserAuthorization{State: UserStateActive}
		if err := r.Scan(&authData.ID, &authData.UserName, &authData.Secret); err != nil {
			return nil, err
		}

		return &authData, nil
	}

	return nil, ErrNoSuchUser
}

func (p *pgxStorage) GetUserAuthInfoByID(userID int64) (*UserAuthorization, error) {
	opCtx, cancel := context.WithTimeout(p.ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, GetUserByIDQuery, userID)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	if r.Next() {
		authData := UserAuthorization{ID: userID, State: UserStateActive}
		if err := r.Scan(&authData.UserName, &authData.Secret); err != nil {
			return nil, err
		}

		return &authData, nil
	}

	return nil, ErrNoSuchUser
}

func (p *pgxStorage) AddOrder(ctx context.Context, userID, orderID int64) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	_, err = tx.Exec(opCtx, AddOrder, orderID, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == UniqueViolationCode && pgErr.ConstraintName == "orders_number_key" {
				r, err := p.dbConn.Query(opCtx, GetOrderUser, orderID)
				if err != nil {
					return err
				}

				if err := r.Err(); err != nil {
					return err
				}
				defer r.Close()

				if r.Next() {
					clientID := int64(0)
					if err := r.Scan(&clientID); err != nil {
						return err
					}
					if clientID == userID {
						return ErrOrderAlreadyPlaced
					}
				}
				return ErrDuplicateOrder
			}
		}
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) GetOrders(ctx context.Context, userID int64) ([]Order, error) {
	return nil, nil
}

func prepareUsersTable(ctx context.Context, conn *pgxpool.Pool) error {
	if r, err := conn.Query(ctx, CheckUsersTable); err == nil {
		r.Close()
		return r.Err()
	}

	r, err := conn.Query(ctx, CreateStateEnum)
	if err != nil {
		return err
	}
	if r.Err() != nil {
		return err
	}
	r.Close()

	r, err = conn.Query(ctx, CreateUsersTableScheme)
	if err != nil {
		return err
	}
	if r.Err() != nil {
		return err
	}
	r.Close()

	r, err = conn.Query(ctx, CreateUserNameIndex)
	if err != nil {
		return err
	}
	r.Close()

	return r.Err()
}

func prepareOrdersTable(ctx context.Context, conn *pgxpool.Pool) error {
	if r, err := conn.Query(ctx, CheckOrdersTable); err == nil {
		r.Close()
		return r.Err()
	}

	r, err := conn.Query(ctx, CreateOrdersTableScheme)
	if err != nil {
		return err
	}
	r.Close()

	return r.Err()
}
