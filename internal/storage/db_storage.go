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

	CreateOrderStatusEnum = `create type order_status as enum ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED');`

	CreateOrdersTableScheme = `
       create table orders (
			number bigint primary key,
			user_id bigint not null,
            status order_status not null default 'NEW',
			accrual double precision not null default 0.0,
			uploaded_at timestamptz not null default now(),
			updated_at timestamptz not null default now(),

			FOREIGN KEY (user_id)
      			REFERENCES users(id)
                ON DELETE CASCADE
		);`

	CheckOrdersTable = `select count(*) from orders;`

	AddOrder            = `insert into orders (number, user_id) values ($1, $2);`
	UpdateOrder         = `update orders set status=$1, accrual=$2, updated_at=now() where number=$3;`
	GetOrderUser        = `select user_id from orders where number = $1;`
	GetUserOrders       = `select number, status, accrual, uploaded_at from orders where user_id = $1;`
	GetUnfinishedOrders = `select number, user_id, status, accrual, uploaded_at from orders where status in ('NEW', 'PROCESSING');`

	CreateBalanceTableScheme = `
       create table balance (
			id bigserial primary key,
			user_id bigint not null,
			current double precision not null default 0 check (current >= 0.0),
			withdrawn double precision not null default 0 check (withdrawn >= 0),
			updated_at timestamptz not null default now(),

			FOREIGN KEY (user_id)
      			REFERENCES users(id)
				ON DELETE CASCADE
		);`

	CheckBalanceTable = `select count(*) from balance;`
	GetUserBalance    = `select current, withdrawn from balance where user_id = $1;`
	SetBalance        = `update balance set current = current-$1, withdrawn=withdrawn+$1 where user_id=$2;`
	AddBalance        = `update balance set current = current+$1 where user_id=$2;`

	CreateWithdrawalTableScheme = `
       create table withdrawal (
			id bigserial primary key,
			number bigint not null unique,
			user_id bigint not null,
			sum double precision not null check (sum >= 0.0),
			processed_at timestamptz not null default now(),

			FOREIGN KEY (user_id)
      			REFERENCES users(id)
				ON DELETE CASCADE
		);`

	CheckWithdrawalTable = `select count(*) from withdrawal;`
	GetUserWithdrawals   = `select number, sum, processed_at from withdrawal where user_id = $1;`
	AddWithdrawal        = `insert into withdrawal (number, user_id, sum) values ($1, $2, $3);`

	CreateUserRelationsFunction = `
		CREATE OR REPLACE FUNCTION function_create_user_relations() RETURNS TRIGGER AS
			$BODY$
			BEGIN
				insert into
					balance (user_id)
					VALUES(new.id);
			
				RETURN new;
			END;
			$BODY$
			language plpgsql;	
	`

	CreateUserRelationsTrigger = `
		create trigger create_user_data
			after insert on users
			for each row
			execute procedure function_create_user_relations();
	`

	DatabaseOperationTimeout = 15 * time.Second

	UniqueViolationCode = "23505"
)

type pgxStorage struct {
	ctx    context.Context
	dbConn *pgxpool.Pool
}

func NewDatabaseStorage(ctx context.Context, connection *pgxpool.Pool) (AppStorage, error) {
	if err := connection.Ping(ctx); err != nil {
		return nil, err
	}

	if err := prepareUsersTable(ctx, connection); err != nil {
		return nil, err
	}

	if err := prepareOrdersTable(ctx, connection); err != nil {
		return nil, err
	}

	if err := prepareBalanceTable(ctx, connection); err != nil {
		return nil, err
	}

	if err := prepareWithdrawalTable(ctx, connection); err != nil {
		return nil, err
	}

	storage := &pgxStorage{
		ctx:    ctx,
		dbConn: connection,
	}
	return storage, nil
}

func (p *pgxStorage) AddUser(ctx context.Context, auth *UserAuthorization) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
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

func (p *pgxStorage) GetUserAuthInfo(ctx context.Context, userName string) (*UserAuthorization, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
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

func (p *pgxStorage) GetUserAuthInfoByID(ctx context.Context, userID int64) (*UserAuthorization, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
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
			if pgErr.Code == UniqueViolationCode && pgErr.ConstraintName == "orders_pkey" {
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

func (p *pgxStorage) UpdateOrder(ctx context.Context, order Order) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	_, err = tx.Exec(opCtx, UpdateOrder, order.Status, order.Accrual, order.ID)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) GetOrders(ctx context.Context, userID int64) ([]Order, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, GetUserOrders, userID)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	orders := make([]Order, 0)
	for r.Next() {
		order := Order{
			UserID: userID,
		}
		if err := r.Scan(&order.ID, &order.Status, &order.Accrual, &order.UploadedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func (p *pgxStorage) GetUnfinishedOrders(ctx context.Context) ([]Order, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, GetUnfinishedOrders)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	orders := make([]Order, 0)
	for r.Next() {
		order := Order{}
		if err := r.Scan(&order.ID, &order.UserID, &order.Status, &order.Accrual, &order.UploadedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func (p *pgxStorage) Withdraw(ctx context.Context, userID, order int64, sum float64) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	r, err := tx.Query(opCtx, GetUserBalance, userID)
	if err != nil {
		return err
	}

	if err := r.Err(); err != nil {
		return err
	}

	defer r.Close()

	info := BalanceInfo{}
	if r.Next() {
		if err := r.Scan(&info.Current, &info.Withdrawn); err != nil {
			return err
		}
	}

	if info.Current-sum < 0 {
		return ErrNotEnoughBalance
	}

	r.Close()

	_, err = tx.Exec(opCtx, AddWithdrawal, order, userID, sum)
	if err != nil {
		return err
	}

	_, err = tx.Exec(opCtx, SetBalance, sum, userID)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) AddBalance(ctx context.Context, userID int64, amount float64) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	_, err = tx.Exec(opCtx, AddBalance, amount, userID)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) UpdateBalanceFromOrders(ctx context.Context, orders []Order) error {
	if len(orders) == 0 {
		return nil
	}

	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	tx, err := p.dbConn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(p.ctx)

	totalAmount := make(map[int64]float64)
	for _, o := range orders {
		_, err = tx.Exec(opCtx, UpdateOrder, o.Status, o.Accrual, o.ID)
		if err != nil {
			return err
		}
		totalAmount[o.UserID] += o.Accrual
	}

	for id, amount := range totalAmount {
		_, err = tx.Exec(opCtx, AddBalance, amount, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit(opCtx)
}

func (p *pgxStorage) GetBalance(ctx context.Context, userID int64) (*BalanceInfo, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, GetUserBalance, userID)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	info := BalanceInfo{}
	if r.Next() {
		if err := r.Scan(&info.Current, &info.Withdrawn); err != nil {
			return nil, err
		}
	}

	return &info, nil
}

func (p *pgxStorage) GetWithdrawals(ctx context.Context, userID int64) ([]Withdrawal, error) {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	r, err := p.dbConn.Query(opCtx, GetUserWithdrawals, userID)

	if err != nil {
		return nil, err
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	defer r.Close()

	ws := make([]Withdrawal, 0)
	for r.Next() {
		w := Withdrawal{}
		if err := r.Scan(&w.Order, &w.Sum, &w.ProcessedAt); err != nil {
			return nil, err
		}
		ws = append(ws, w)
	}

	return ws, nil
}

func prepareUsersTable(ctx context.Context, conn *pgxpool.Pool) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	_, err := conn.Exec(opCtx, CheckUsersTable)
	if err == nil {
		return nil
	}

	tx, err := conn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(opCtx, CreateStateEnum)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, CreateUsersTableScheme)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, CreateUserNameIndex)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func prepareOrdersTable(ctx context.Context, conn *pgxpool.Pool) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	_, err := conn.Exec(opCtx, CheckOrdersTable)
	if err == nil {
		return nil
	}

	tx, err := conn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(opCtx, CreateOrderStatusEnum)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, CreateOrdersTableScheme)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func prepareBalanceTable(ctx context.Context, conn *pgxpool.Pool) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	_, err := conn.Exec(opCtx, CheckBalanceTable)
	if err == nil {
		return nil
	}

	tx, err := conn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(opCtx, CreateBalanceTableScheme)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, CreateUserRelationsFunction)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, CreateUserRelationsTrigger)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}

func prepareWithdrawalTable(ctx context.Context, conn *pgxpool.Pool) error {
	opCtx, cancel := context.WithTimeout(ctx, DatabaseOperationTimeout)
	defer cancel()

	_, err := conn.Exec(opCtx, CheckWithdrawalTable)
	if err == nil {
		return nil
	}

	tx, err := conn.Begin(opCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(opCtx, CreateWithdrawalTableScheme)
	if err != nil {
		return err
	}

	return tx.Commit(opCtx)
}
