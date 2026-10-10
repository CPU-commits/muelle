// Package http tiene los endpoints de variables de stages y apps (§10 del spec).
package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/app/usecases"
	"github.com/CPU-commits/muelle/src/internal/secrets/infra/in/form"
)

const (
	stageEnvPath = "/api/v1/projects/{p}/stages/{s}/env"
	appEnvPath   = "/api/v1/projects/{p}/stages/{s}/apps/{a}/env"
)

// EnvHandler atiende GET, PUT y PATCH de variables. Los mismos handlers sirven
// para los dos dueños: en la ruta del stage, {a} viene vacío.
type EnvHandler struct {
	get     *usecases.GetEnv
	replace *usecases.ReplaceEnv
	patch   *usecases.PatchEnv
}

func NewEnvHandler(get *usecases.GetEnv, replace *usecases.ReplaceEnv, patch *usecases.PatchEnv) *EnvHandler {
	return &EnvHandler{get: get, replace: replace, patch: patch}
}

// Register registra las rutas. Todas aceptan tokens de API: el middleware del
// Kit revisa el alcance del token con {p} y {s} (§10.1 del spec).
func (h *EnvHandler) Register(k *utils.Kit) {
	for _, path := range []string{stageEnvPath, appEnvPath} {
		k.Handle("GET "+path, utils.AccessToken, h.getEnv(k))
		k.Handle("PUT "+path, utils.AccessToken, h.putEnv(k))
		k.Handle("PATCH "+path, utils.AccessToken, h.patchEnv(k))
	}
}

func target(r *http.Request) dto.EnvTarget {
	return dto.EnvTarget{Project: r.PathValue("p"), Stage: r.PathValue("s"), App: r.PathValue("a")}
}

func (h *EnvHandler) getEnv(k *utils.Kit) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars, err := h.get.Execute(r.Context(), target(r))
		if err != nil {
			k.Fail(w, err)
			return
		}
		utils.WriteJSON(w, http.StatusOK, newEnvRes(vars))
	}
}

// putEnv reemplaza todo el set, como el sync-env de GitHub Actions.
func (h *EnvHandler) putEnv(k *utils.Kit) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in form.ReplaceEnv
		if !utils.DecodeJSON(w, r, &in) {
			return
		}
		if err := h.replace.Execute(r.Context(), in.DTO(target(r))); err != nil {
			k.Fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *EnvHandler) patchEnv(k *utils.Kit) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in form.PatchEnv
		if !utils.DecodeJSON(w, r, &in) {
			return
		}
		if err := h.patch.Execute(r.Context(), in.DTO(target(r))); err != nil {
			k.Fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
