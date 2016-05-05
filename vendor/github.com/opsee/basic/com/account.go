package com

import (
	"fmt"
	"time"
)

type Account struct {
	ID         int       `json:"id" db:"id"`
	ExternalID string    `json:"external_id" db:"external_id"`
	CustomerID string    `json:"customer_id" db:"customer_id"`
	RoleARN    string    `json:"role_arn" db:"role_arn"`
	Active     bool      `json:"active" db:"active"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

func (a *Account) RoleName() string {
	return fmt.Sprintf("opsee-role-%s", a.CustomerID)
}

func (a *Account) PolicyName() string {
	return fmt.Sprintf("opsee-policy-%s", a.CustomerID)
}

func (a *Account) ComputedRoleARN() string {
	return fmt.Sprintf("arn:aws:iam::%012d:role/%s", a.ID, a.RoleName())
}

func (a *Account) PolicyARN() string {
	return fmt.Sprintf("arn:aws:iam::%012d:policy/%s", a.ID, a.PolicyName())
}
