// Package form tiene los cuerpos JSON que reciben los endpoints de identity.
// Se leen con utils.DecodeJSON (json/v2 estricto: los nombres de los campos
// se comparan exactamente y un campo desconocido es un error).
package form

import "github.com/CPU-commits/muelle/src/internal/identity/app/dto"

// Credentials es el cuerpo de POST /auth/login y POST /auth/invite/{token}.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (f Credentials) Login(ip, userAgent string) dto.LoginInput {
	return dto.LoginInput{Username: f.Username, Password: f.Password, IP: ip, UserAgent: userAgent}
}

func (f Credentials) AcceptInvite(token, ip, userAgent string) dto.AcceptInviteInput {
	return dto.AcceptInviteInput{Token: token, Username: f.Username, Password: f.Password, IP: ip, UserAgent: userAgent}
}

// Setup es el cuerpo de POST /auth/setup: el token del link que imprime `muelle serve`.
type Setup struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (f Setup) AcceptInvite(ip, userAgent string) dto.AcceptInviteInput {
	return dto.AcceptInviteInput{Token: f.Token, Username: f.Username, Password: f.Password, IP: ip, UserAgent: userAgent}
}

// Code es el cuerpo de POST /auth/totp y POST /auth/totp/enable.
type Code struct {
	Code string `json:"code"`
}

// CreateToken es el cuerpo de POST /tokens. Sin stage, el token es de todo
// el proyecto; expires_in_days 0 u omitido, no vence. read_only y
// all_projects son opcionales (contrato §8.1): un cliente que no los manda
// crea un token de escritura de un proyecto, como antes.
type CreateToken struct {
	Name          string `json:"name"`
	Project       string `json:"project"`
	Stage         string `json:"stage"`
	ReadOnly      bool   `json:"read_only"`
	AllProjects   bool   `json:"all_projects"`
	ExpiresInDays int    `json:"expires_in_days"`
}

func (f CreateToken) DTO(createdBy int64) dto.CreateTokenInput {
	return dto.CreateTokenInput{Name: f.Name, Project: f.Project, Stage: f.Stage, ReadOnly: f.ReadOnly,
		AllProjects: f.AllProjects, ExpiresInDays: f.ExpiresInDays, CreatedBy: createdBy}
}
