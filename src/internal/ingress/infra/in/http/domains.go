package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
)

// appRef son los slugs de {app} en la ruta.
func appRef(r *http.Request) dto.AppRef {
	return dto.AppRef{Project: r.PathValue("p"), Stage: r.PathValue("s"), App: r.PathValue("a")}
}

// domainRef es el dominio de la ruta ({hostname} ya viene sin url-encoding).
func domainRef(r *http.Request) dto.DomainRef {
	return dto.DomainRef{AppRef: appRef(r), Hostname: r.PathValue("hostname")}
}

// listDomains es GET {app}/domains (contrato §3.2).
func (h *handler) listDomains(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.ListDomains.Execute(r.Context(), appRef(r))
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

// createDomain es POST {app}/domains: 201 con el dominio.
func (h *handler) createDomain(w http.ResponseWriter, r *http.Request) {
	var in dto.CreateDomainInput
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	in.AppRef = appRef(r)
	out, err := h.uc.CreateDomain.Execute(r.Context(), in)
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, out)
}

// updateDomain es PATCH {app}/domains/{hostname}: el hostname no se edita
// (mandarlo es un campo desconocido, 400).
func (h *handler) updateDomain(w http.ResponseWriter, r *http.Request) {
	var in dto.UpdateDomainInput
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	ref := domainRef(r)
	in.AppRef, in.Hostname = ref.AppRef, ref.Hostname
	out, err := h.uc.UpdateDomain.Execute(r.Context(), in)
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

// deleteDomain es DELETE {app}/domains/{hostname}: 204 sin cuerpo.
func (h *handler) deleteDomain(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.DeleteDomain.Execute(r.Context(), domainRef(r)); err != nil {
		h.k.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adoptDomain es POST {app}/domains/{hostname}/adopt, sin cuerpo o con {}.
func (h *handler) adoptDomain(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyBody(w, r) {
		return
	}
	out, err := h.uc.AdoptDomain.Execute(r.Context(), domainRef(r))
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

// retryDomain es POST {app}/domains/{hostname}/retry, sin cuerpo o con {}.
func (h *handler) retryDomain(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyBody(w, r) {
		return
	}
	out, err := h.uc.RetryDomain.Execute(r.Context(), domainRef(r))
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}
