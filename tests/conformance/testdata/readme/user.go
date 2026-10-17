// Package readme is the README's first example, kept runnable so the README
// shows what the code prints.
package readme

import "errors"

// ErrDuplicate reports an email that is already registered.
var ErrDuplicate = errors.New("duplicate user")

// UserService registers users by email.
type UserService struct{ users map[string]bool }

// NewUserService returns an empty service.
func NewUserService() *UserService { return &UserService{users: map[string]bool{}} }

// Create registers an email once.
func (u *UserService) Create(email string) error {
	if u.users[email] {
		return ErrDuplicate
	}
	u.users[email] = true
	return nil
}
