package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/infra/in/form"
)

func (h *handler) setSession(w http.ResponseWriter, s dto.SessionOutput) {
	h.k.SetSessionCookie(w, s.Token, s.ExpiresAt)
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var in form.Credentials
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	s, err := h.uc.Login.Execute(r.Context(), in.Login(utils.ClientIP(r), r.UserAgent()))
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	h.setSession(w, s)
	utils.WriteJSON(w, http.StatusOK, mfaRes{MFARequired: s.MFARequired})
}

// completeTOTP cambia la sesión pendiente de MFA por una completa, con otro token.
func (h *handler) completeTOTP(w http.ResponseWriter, r *http.Request) {
	var in form.Code
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	s, err := h.uc.CompleteTOTP.Execute(r.Context(), dto.CompleteTOTPInput{
		SessionHash: principal(r).SessionHash, Code: in.Code, IP: utils.ClientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	h.setSession(w, s)
	utils.WriteJSON(w, http.StatusOK, mfaRes{MFARequired: false})
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.Logout.Execute(r.Context(), principal(r).SessionHash); err != nil {
		h.k.Fail(w, err)
		return
	}
	h.k.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	me, err := h.uc.Me.Execute(r.Context(), principal(r))
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, toMeRes(me))
}

func (h *handler) beginTOTPSetup(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.BeginTOTPSetup.Execute(r.Context(), principal(r).UserID)
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, totpSetupRes{Secret: out.Secret, URL: out.URL})
}

func (h *handler) enableTOTP(w http.ResponseWriter, r *http.Request) {
	var in form.Code
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	if err := h.uc.EnableTOTP.Execute(r.Context(), dto.EnableTOTPInput{UserID: principal(r).UserID, Code: in.Code}); err != nil {
		h.k.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setup crea el primer usuario con el link que imprime `muelle serve`.
func (h *handler) setup(w http.ResponseWriter, r *http.Request) {
	var in form.Setup
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	s, err := h.uc.Setup.Execute(r.Context(), in.AcceptInvite(utils.ClientIP(r), r.UserAgent()))
	h.created(w, s, err)
}

func (h *handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var in form.Credentials
	if !utils.DecodeJSON(w, r, &in) {
		return
	}
	s, err := h.uc.AcceptInvite.Execute(r.Context(), in.AcceptInvite(r.PathValue("token"), utils.ClientIP(r), r.UserAgent()))
	h.created(w, s, err)
}

// created responde a un usuario recién creado, con su sesión.
func (h *handler) created(w http.ResponseWriter, s dto.SessionOutput, err error) {
	if err != nil {
		h.k.Fail(w, err)
		return
	}
	h.setSession(w, s)
	utils.WriteJSON(w, http.StatusCreated, usernameRes{Username: s.Username})
}
