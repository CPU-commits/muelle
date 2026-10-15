package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/deploy/infra/in/form"
)

// Los registries (contrato §2.10), solo con sesión: un token recibe 403
// token_not_allowed. El host va url-encoded en la ruta.

func (h *Handler) listRegistries(w http.ResponseWriter, r *http.Request) {
	list, err := h.uc.ListRegistries.Execute(r.Context())
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, newRegistriesRes(list))
}

func (h *Handler) createRegistry(w http.ResponseWriter, r *http.Request) {
	var in form.CreateRegistry
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	reg, err := h.uc.CreateRegistry.Execute(r.Context(), in.DTO())
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, newRegistryRes(reg))
}

func (h *Handler) updateRegistry(w http.ResponseWriter, r *http.Request) {
	var in form.UpdateRegistry
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	reg, err := h.uc.UpdateRegistry.Execute(r.Context(), in.DTO(r.PathValue("host")))
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, newRegistryRes(reg))
}

func (h *Handler) deleteRegistry(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.DeleteRegistry.Execute(r.Context(), r.PathValue("host")); err != nil {
		h.k.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
