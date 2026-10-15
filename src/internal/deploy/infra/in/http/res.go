package http

import (
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
)

// RegistryRes es un registry en la respuesta (contrato §2.10): nunca la contraseña.
type RegistryRes struct {
	Host      string    `json:"host"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newRegistryRes(r entity.Registry) RegistryRes {
	return RegistryRes{Host: r.Host, Username: r.Username, CreatedAt: millis(r.CreatedAt), UpdatedAt: millis(r.UpdatedAt)}
}

func newRegistriesRes(list []entity.Registry) []RegistryRes {
	out := make([]RegistryRes, 0, len(list))
	for _, r := range list {
		out = append(out, newRegistryRes(r))
	}
	return out
}

// millis es t en UTC truncada al milisegundo (§10 del diseño).
func millis(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }
