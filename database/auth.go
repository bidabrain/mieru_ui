package database

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

const pbkdf2Iter = 210000

func hashPassword(password, salt string) (string, error) {
	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// SetPassword stores a salted PBKDF2 hash of password in s.
func SetPassword(s *Settings, password string) error {
	salt := RandomHex(16)
	h, err := hashPassword(password, salt)
	if err != nil {
		return err
	}
	s.PasswordSalt = salt
	s.PasswordHash = h
	return nil
}

// CheckLogin validates the admin credentials.
func CheckLogin(s Settings, username, password string) bool {
	h, err := hashPassword(password, s.PasswordSalt)
	if err != nil {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(s.Username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(h), []byte(s.PasswordHash)) == 1
	return userOK && passOK
}
