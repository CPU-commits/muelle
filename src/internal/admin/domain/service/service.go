// Package service define los errores y los puertos de servicios del módulo admin.
package service

import (
	"context"
	"errors"
)

var (
	ErrWrongMasterKey = errors.New("clave maestra incorrecta")
	// ErrTOTPRequiredFirst: quien activa require_2fa sin tener su propio TOTP quedaría encerrado (§10.1).
	ErrTOTPRequiredFirst = errors.New("configura tu propio 2FA antes de exigirlo a todos")
)

// TOTPStatus le pregunta a identity (vía SQL, sin importarlo) si un usuario tiene 2FA.
type TOTPStatus interface {
	HasTOTP(ctx context.Context, userID int64) (bool, error)
}

// BackupStore guarda las copias de la base. Los nombres son solo el nombre
// del archivo y ordenan cronológicamente (muelle-AAAA-MM-DD.db).
type BackupStore interface {
	// Create escribe una copia consistente de la base con ese nombre. Si falla no deja un archivo a medias.
	Create(ctx context.Context, name string) error
	// List devuelve los nombres de las copias, de la más vieja a la más nueva.
	List(ctx context.Context) ([]string, error)
	Remove(ctx context.Context, name string) error
}
