package http

import (
	"net/http"
	"strconv"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/infra/in/form"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

func (h *handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.uc.ListUsers.Execute(r.Context())
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	out := make([]userRes, 0, len(users))
	for _, u := range users {
		out = append(out, toUserRes(u))
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	in := dto.DeleteUserInput{Username: r.PathValue("username"), ActorUsername: principal(r).Username}
	if err := h.uc.DeleteUser.Execute(r.Context(), in); err != nil {
		h.k.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resetUserTOTP sirve cuando alguien pierde su teléfono.
func (h *handler) resetUserTOTP(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.ResetUserTOTP.Execute(r.Context(), r.PathValue("username")); err != nil {
		h.k.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) createInvite(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	inv, err := h.uc.CreateInvite.Execute(r.Context(), dto.CreateInviteInput{CreatedBy: p.UserID, CreatorUsername: p.Username})
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	// El link usa el Origin de la petición: cmd/http rechaza (403 bad_origin)
	// todo POST sin Bearer cuyo Origin no sea exactamente el del panel, y esta
	// ruta no acepta tokens, así que aquí Origin siempre es el del panel.
	url := r.Header.Get("Origin") + "/invite/" + inv.Token
	utils.WriteJSON(w, http.StatusCreated, inviteRes{URL: url, ExpiresAt: inv.ExpiresAt})
}

func (h *handler) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.uc.ListTokens.Execute(r.Context())
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	out := make([]tokenRes, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, toTokenRes(t))
	}
	utils.WriteJSON(w, http.StatusOK, out)
}

func (h *handler) createToken(w http.ResponseWriter, r *http.Request) {
	var in form.CreateToken
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	t, err := h.uc.CreateToken.Execute(r.Context(), in.DTO(principal(r).UserID))
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, tokenCreatedRes{tokenRes: toTokenRes(t.TokenOutput), Token: t.Plain})
}

func (h *handler) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.k.Fail(w, validate.Errorf("id", "debe ser un número"))
		return
	}
	if err := h.uc.DeleteToken.Execute(r.Context(), id); err != nil {
		h.k.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
