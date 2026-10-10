package repository

import "context"

// Transactor ejecuta fn en una transacción. Los repositorios que reciben el
// ctx de fn participan en ella. Si fn devuelve error se hace rollback.
// Llamar a InTx dentro de fn reutiliza la transacción en curso.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}
