package store

import (
	"database/sql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/opsee/basic/com"
)

type Postgres struct {
	db *sqlx.DB
}

func NewPostgres(connection string) (Store, error) {
	db, err := sqlx.Open("postgres", connection)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(64)
	db.SetMaxIdleConns(8)

	return &Postgres{
		db: db,
	}, nil
}

func (pg *Postgres) PutAccount(account *com.Account) error {
	return pg.putAccount(pg.db, account)
}

func (pg *Postgres) UpdateAccount(account *com.Account) error {
	return pg.updateAccount(pg.db, account)
}

func (pg *Postgres) DeleteAccount(account *com.Account) error {
	return pg.deleteAccount(pg.db, account)
}

func (pg *Postgres) GetAccount(request *GetAccountRequest) (*com.Account, error) {
	account := &com.Account{}
	err := pg.db.Get(
		account,
		"select * from accounts where customer_id = $1 and active = $2 limit 1",
		request.CustomerID,
		request.Active,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return account, nil
}

func (pg *Postgres) putAccount(x sqlx.Ext, account *com.Account) error {
	_, err := sqlx.NamedExec(
		x,
		`insert into accounts (id, customer_id, active) values (:id, :customer_id, :active)`,
		account,
	)
	return err
}

func (pg *Postgres) updateAccount(x sqlx.Ext, account *com.Account) error {
	_, err := sqlx.NamedExec(
		x,
		`update accounts set active = :active where id = :id and customer_id = :customer_id`,
		account,
	)
	return err
}

func (pg *Postgres) deleteAccount(x sqlx.Ext, account *com.Account) error {
	_, err := sqlx.NamedExec(
		x,
		`delete from accounts where id = :id and customer_id = :customer_id`,
		account,
	)
	return err
}
