package store

import (
	"github.com/opsee/basic/com"
)

type Store interface {
	PutAccount(*com.Account) error
	UpdateAccount(*com.Account, *com.Account) error
	DeleteAccount(*com.Account) error

	GetAccount(*GetAccountRequest) (*com.Account, error)
}

type GetAccountRequest struct {
	CustomerID string
	Active     bool
}
