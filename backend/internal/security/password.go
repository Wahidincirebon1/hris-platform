package security

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordBytes = 12
	MaxPasswordBytes = 72
)

func HashPassword(password string) (string, error) {
	if len([]byte(password)) < MinPasswordBytes || len([]byte(password)) > MaxPasswordBytes {
		return "", errors.New("password must contain 12 to 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
