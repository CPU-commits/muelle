package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
)

// ListUsers devuelve los usuarios ordenados por nombre.
type ListUsers struct {
	users repository.UserRepository
}

func NewListUsers(users repository.UserRepository) *ListUsers { return &ListUsers{users: users} }

func (uc *ListUsers) Execute(ctx context.Context) ([]dto.UserOutput, error) {
	users, err := uc.users.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.UserOutput, 0, len(users))
	for _, u := range users {
		out = append(out, dto.UserOutput{Username: u.Username, TOTPEnabled: u.HasTOTP(), CreatedAt: u.CreatedAt})
	}
	return out, nil
}
