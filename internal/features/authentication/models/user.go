package models

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type UserStatus string

const (
	UserStatusPending   UserStatus = "PENDING"
	UserStatusActive    UserStatus = "ACTIVE"
	UserStatusSuspended UserStatus = "SUSPENDED"
)

type User struct {
	ID        string
	FirstName string
	LastName  string
	Email     string
	Status    UserStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewUser(
	id string,
	firstName string,
	lastName string,
	email string,
	status UserStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (*User, error) {
	id = strings.TrimSpace(id)
	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	email = strings.TrimSpace(email)

	if !IsUUID(id) {
		return nil, errors.New("user ID must be a valid UUID")
	}
	if firstName == "" {
		return nil, errors.New("first name is required")
	}
	if lastName == "" {
		return nil, errors.New("last name is required")
	}
	return &User{
		ID:        id,
		Status:    status,
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func IsUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}
