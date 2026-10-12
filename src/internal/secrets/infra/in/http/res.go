package http

import (
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
)

// EnvVarRes es una variable en la respuesta. Value es null si es secreta.
type EnvVarRes struct {
	Key       string    `json:"key"`
	Value     *string   `json:"value"`
	Secret    bool      `json:"secret"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EnvRes es la respuesta de GET …/env.
type EnvRes struct {
	Vars []EnvVarRes `json:"vars"`
}

func newEnvRes(vars []service.MaskedVar) EnvRes {
	out := make([]EnvVarRes, 0, len(vars))
	for _, v := range vars {
		out = append(out, EnvVarRes{Key: v.Key, Value: v.Value, Secret: v.Secret, UpdatedAt: v.UpdatedAt})
	}
	return EnvRes{Vars: out}
}
