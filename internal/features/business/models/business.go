package models

import "time"

type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusSuspended Status = "SUSPENDED"
)

type Role string

const RoleOwner Role = "OWNER"

type Business struct {
	ID        string
	Name      string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Membership struct {
	ID         string
	BusinessID string
	UserID     string
	Role       Role
	CreatedAt  time.Time
}
