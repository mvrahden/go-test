package sampletype

import "context"

type UserService struct{}

func NewUserService() *UserService { return &UserService{} }

func (s *UserService) Create(ctx context.Context, name string) error { return nil }
func (s *UserService) GetByID(ctx context.Context, id string) (*UserService, error) {
	return nil, nil
}
func (s *UserService) Delete(ctx context.Context, id string) error { return nil }
func (s *UserService) unexported()                                 {}

type Validator interface {
	Validate(input string) error
	IsValid(input string) bool
}
