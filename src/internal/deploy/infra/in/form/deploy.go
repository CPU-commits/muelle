package form

import "github.com/CPU-commits/muelle/src/internal/deploy/app/dto"

// CreateDeploy es el cuerpo de POST …/deploys (contrato §2.2), opcional:
// vacío es lo mismo que {}.
type CreateDeploy struct {
	Image  *string           `json:"image"`
	Images map[string]string `json:"images"`
}

func (f CreateDeploy) DTO(ref dto.AppRef) dto.CreateDeploy {
	return dto.CreateDeploy{AppRef: ref, Image: f.Image, Images: f.Images}
}

// Rollback es el cuerpo de POST …/rollback (contrato §2.7): ninguno, o {}.
type Rollback struct{}
