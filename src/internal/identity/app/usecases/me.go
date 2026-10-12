package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Me describe al usuario de la sesión.
type Me struct {
	users repository.UserRepository
}

func NewMe(users repository.UserRepository) *Me { return &Me{users: users} }

func (uc *Me) Execute(ctx context.Context, p shared.Principal) (dto.MeOutput, error) {
	u, err := uc.users.GetByID(ctx, p.UserID)
	if err != nil {
		return dto.MeOutput{}, err
	}
	return dto.MeOutput{Username: u.Username, TOTPEnabled: u.HasTOTP(), NeedsTOTPSetup: p.NeedsTOTPSetup}, nil
}
