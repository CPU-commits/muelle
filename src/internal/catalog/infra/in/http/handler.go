// Package http expone los casos de uso del catálogo como endpoints REST.
package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/catalog/app/dto"
	"github.com/CPU-commits/muelle/src/internal/catalog/app/usecases"
)

// UseCases son los casos de uso que usan los handlers.
type UseCases struct {
	CreateProject *usecases.CreateProject
	ListProjects  *usecases.ListProjects
	GetProject    *usecases.GetProject
	UpdateProject *usecases.UpdateProject
	DeleteProject *usecases.DeleteProject

	CreateStage *usecases.CreateStage
	ListStages  *usecases.ListStages
	GetStage    *usecases.GetStage
	DeleteStage *usecases.DeleteStage

	CreateApp *usecases.CreateApp
	ListApps  *usecases.ListApps
	GetApp    *usecases.GetApp
	UpdateApp *usecases.UpdateApp
	DeleteApp *usecases.DeleteApp
}

type Handler struct {
	uc UseCases
	k  *utils.Kit
}

// Register registra los errores del módulo y todas sus rutas en k.
//
// Los GET de proyecto, stages y apps aceptan tokens de API (AccessToken; el
// Kit verifica su alcance con {p} y {s}). Todo lo demás, incluido listar los
// proyectos, exige sesión.
func Register(k *utils.Kit, uc UseCases) {
	registerErrors(k)
	h := &Handler{uc: uc, k: k}

	k.Handle("GET /api/v1/projects", utils.AccessSession, h.listProjects)
	k.Handle("POST /api/v1/projects", utils.AccessSession, h.createProject)
	k.Handle("GET /api/v1/projects/{p}", utils.AccessToken, h.getProject)
	k.Handle("PATCH /api/v1/projects/{p}", utils.AccessSession, h.updateProject)
	k.Handle("DELETE /api/v1/projects/{p}", utils.AccessSession, h.deleteProject)

	k.Handle("GET /api/v1/projects/{p}/stages", utils.AccessToken, h.listStages)
	k.Handle("POST /api/v1/projects/{p}/stages", utils.AccessSession, h.createStage)
	k.Handle("GET /api/v1/projects/{p}/stages/{s}", utils.AccessToken, h.getStage)
	k.Handle("DELETE /api/v1/projects/{p}/stages/{s}", utils.AccessSession, h.deleteStage)

	const apps = "/api/v1/projects/{p}/stages/{s}/apps"
	k.Handle("GET "+apps, utils.AccessToken, h.listApps)
	k.Handle("POST "+apps, utils.AccessSession, h.createApp)
	k.Handle("GET "+apps+"/{a}", utils.AccessToken, h.getApp)
	k.Handle("PATCH "+apps+"/{a}", utils.AccessSession, h.updateApp)
	k.Handle("DELETE "+apps+"/{a}", utils.AccessSession, h.deleteApp)
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.ListProjects.Execute(r.Context())
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var in dto.CreateProjectInput
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	out, err := h.uc.CreateProject.Execute(r.Context(), in)
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.GetProject.Execute(r.Context(), r.PathValue("p"))
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	in := dto.UpdateProjectInput{Slug: r.PathValue("p")}
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	out, err := h.uc.UpdateProject.Execute(r.Context(), in)
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.DeleteProject.Execute(r.Context(), r.PathValue("p")); err != nil {
		h.k.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listStages(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.ListStages.Execute(r.Context(), r.PathValue("p"))
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createStage(w http.ResponseWriter, r *http.Request) {
	in := dto.CreateStageInput{Project: r.PathValue("p")}
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	out, err := h.uc.CreateStage.Execute(r.Context(), in)
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) getStage(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.GetStage.Execute(r.Context(), r.PathValue("p"), r.PathValue("s"))
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteStage(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.DeleteStage.Execute(r.Context(), r.PathValue("p"), r.PathValue("s")); err != nil {
		h.k.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listApps(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.ListApps.Execute(r.Context(), r.PathValue("p"), r.PathValue("s"))
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createApp(w http.ResponseWriter, r *http.Request) {
	in := dto.CreateAppInput{Project: r.PathValue("p"), Stage: r.PathValue("s")}
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	out, err := h.uc.CreateApp.Execute(r.Context(), in)
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) getApp(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.GetApp.Execute(r.Context(), r.PathValue("p"), r.PathValue("s"), r.PathValue("a"))
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) updateApp(w http.ResponseWriter, r *http.Request) {
	in := dto.UpdateAppInput{Project: r.PathValue("p"), Stage: r.PathValue("s"), Slug: r.PathValue("a")}
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	out, err := h.uc.UpdateApp.Execute(r.Context(), in)
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteApp(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.DeleteApp.Execute(r.Context(), r.PathValue("p"), r.PathValue("s"), r.PathValue("a")); err != nil {
		h.k.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
