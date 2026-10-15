package http

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/infra/in/form"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// Los deploys de una app (contrato §2.1–§2.5 y §2.7). Todas las rutas
// aceptan tokens dentro de su alcance (el Kit lo verifica con {p} y {s}).

func appRef(r *http.Request) dto.AppRef {
	return dto.AppRef{Project: r.PathValue("p"), Stage: r.PathValue("s"), App: r.PathValue("a")}
}

// deployRef lee {id}: uno que no es un entero > 0 no existe (404).
func deployRef(r *http.Request) (dto.DeployRef, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return dto.DeployRef{}, repository.ErrNotFound
	}
	return dto.DeployRef{AppRef: appRef(r), ID: id}, nil
}

// ParseOffset lee offset de la query (contrato §2.5): ausente es 0; si
// viene, tiene que ser un entero ≥ 0 en decimal, y si no lo es devuelve -1
// (el caso de uso responde 422 offset, después de los 404).
func ParseOffset(q url.Values) int {
	if !q.Has("offset") {
		return 0
	}
	return parseOffset(q.Get("offset"))
}

func parseOffset(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || s == "" || strings.Trim(s, "0123456789") != "" {
		return -1
	}
	return n
}

// readOptionalBody lee un cuerpo que puede faltar (contrato §2.2 y §2.7):
// vacío o solo espacios deja dst como está y sigue; si no, lo decodifica con
// utils.DecodeJSON (estricto, hasta 1 MiB) y, si no sirve, ya respondió 400.
// DecodeJSON solo no sirve: con un cuerpo vacío responde 400, y el panel
// manda el rollback sin cuerpo.
func readOptionalBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	// Un byte más que el tope: si lo pasa, DecodeJSON responde "el cuerpo supera 1 MiB".
	b, err := io.ReadAll(io.LimitReader(r.Body, utils.MaxBodyBytes+1))
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid_json", "JSON inválido: no se pudo leer el cuerpo")
		return false
	}
	if len(bytes.Trim(b, " \t\r\n")) == 0 {
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	return utils.DecodeJSON(w, r, dst)
}

func (h *Handler) createDeploy(w http.ResponseWriter, r *http.Request) {
	ref := appRef(r)
	// El orden de los errores (contrato §2.2): app (404), cuerpo (400) y validación (422).
	if err := h.uc.CreateDeploy.CheckApp(r.Context(), ref); err != nil {
		h.k.Fail(w, r, err)
		return
	}
	var in form.CreateDeploy
	if !readOptionalBody(w, r, &in) {
		return
	}
	q, err := h.uc.CreateDeploy.Execute(r.Context(), in.DTO(ref))
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", q.StatusURL)
	utils.WriteJSON(w, http.StatusAccepted, newQueuedRes(q))
}

func (h *Handler) rollback(w http.ResponseWriter, r *http.Request) {
	ref := appRef(r)
	if err := h.uc.Rollback.CheckApp(r.Context(), ref); err != nil {
		h.k.Fail(w, r, err)
		return
	}
	var in form.Rollback
	if !readOptionalBody(w, r, &in) {
		return
	}
	q, err := h.uc.Rollback.Execute(r.Context(), ref)
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", q.StatusURL)
	utils.WriteJSON(w, http.StatusAccepted, newQueuedRes(q))
}

func (h *Handler) listDeploys(w http.ResponseWriter, r *http.Request) {
	list, err := h.uc.ListDeploys.Execute(r.Context(), appRef(r))
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, newDeploysRes(list))
}

func (h *Handler) getDeploy(w http.ResponseWriter, r *http.Request) {
	ref, err := deployRef(r)
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	d, err := h.uc.GetDeploy.Execute(r.Context(), ref)
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, newDeployRes(d))
}

func (h *Handler) readLog(w http.ResponseWriter, r *http.Request) {
	ref, err := deployRef(r)
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	// Un offset inválido es -1: el caso de uso responde los 404 antes del 422.
	page, err := h.uc.ReadLog.Execute(r.Context(), dto.ReadLog{DeployRef: ref, Offset: ParseOffset(r.URL.Query())})
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, newLogRes(page))
}
