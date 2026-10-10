package http

import (
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
)

// Respuestas JSON de identity (json/v2: nil en un slice sale como [], y un
// puntero nil como null).

type mfaRes struct {
	MFARequired bool `json:"mfa_required"`
}

type usernameRes struct {
	Username string `json:"username"`
}

type meRes struct {
	Username       string `json:"username"`
	TOTPEnabled    bool   `json:"totp_enabled"`
	NeedsTOTPSetup bool   `json:"needs_totp_setup"`
}

type totpSetupRes struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
}

type userRes struct {
	Username    string    `json:"username"`
	TOTPEnabled bool      `json:"totp_enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

type inviteRes struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type tokenRes struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Project    string     `json:"project"`
	Stage      string     `json:"stage,omitempty"` // sin stage: todo el proyecto
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// tokenCreatedRes lleva el token en claro, que se muestra una sola vez.
type tokenCreatedRes struct {
	tokenRes `json:",inline"`
	Token    string `json:"token"`
}

func toMeRes(m dto.MeOutput) meRes {
	return meRes{Username: m.Username, TOTPEnabled: m.TOTPEnabled, NeedsTOTPSetup: m.NeedsTOTPSetup}
}

func toUserRes(u dto.UserOutput) userRes {
	return userRes{Username: u.Username, TOTPEnabled: u.TOTPEnabled, CreatedAt: u.CreatedAt}
}

func toTokenRes(t dto.TokenOutput) tokenRes {
	return tokenRes{ID: t.ID, Name: t.Name, Project: t.Project, Stage: t.Stage,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt}
}
