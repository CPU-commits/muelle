// Package http expone los endpoints REST del módulo admin.
package http

import (
	"net/http"
	"strconv"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/app/usecases"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

type Handler struct {
	getSettings   *usecases.GetSettings
	patchSettings *usecases.PatchSettings
	listAudit     *usecases.ListAudit
}

func NewHandler(get *usecases.GetSettings, patch *usecases.PatchSettings, list *usecases.ListAudit) *Handler {
	return &Handler{getSettings: get, patchSettings: patch, listAudit: list}
}

// Register agrega las rutas y los errores del módulo al Kit.
func (h *Handler) Register(k *utils.Kit) {
	registerErrors(k)
	k.Handle("GET /api/v1/settings", utils.AccessSession, h.handleGetSettings(k))
	k.Handle("PATCH /api/v1/settings", utils.AccessSession, h.handlePatchSettings(k))
	k.Handle("GET /api/v1/audit", utils.AccessSession, h.handleListAudit(k))
}

func (h *Handler) handleGetSettings(k *utils.Kit) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := h.getSettings.Execute(r.Context())
		if err != nil {
			k.Fail(w, r, err)
			return
		}
		utils.WriteJSON(w, http.StatusOK, out)
	}
}

// handlePatchSettings: en la Fase 1 solo existe require_2fa. Las fases
// siguientes agregan Cloudflare, IP pública, email ACME, etc.
func (h *Handler) handlePatchSettings(k *utils.Kit) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.PatchSettingsInput
		if !utils.DecodeJSON(w, r, &in) {
			return
		}
		out, err := h.patchSettings.Execute(r.Context(), in)
		if err != nil {
			k.Fail(w, r, err)
			return
		}
		utils.WriteJSON(w, http.StatusOK, out)
	}
}

func (h *Handler) handleListAudit(k *utils.Kit) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.ListAuditInput
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				k.Fail(w, r, validate.Errorf("limit", "debe ser un número"))
				return
			}
			in.Limit = &n
		}
		if v := r.URL.Query().Get("before"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				k.Fail(w, r, validate.Errorf("before", "debe ser un id válido"))
				return
			}
			in.Before = &n
		}
		events, err := h.listAudit.Execute(r.Context(), in)
		if err != nil {
			k.Fail(w, r, err)
			return
		}
		utils.WriteJSON(w, http.StatusOK, events)
	}
}
