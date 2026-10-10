package repository

import "context"

// Transactor ejecuta fn en una transacción. Los repositorios que reciben el
// ctx de fn participan en ella. Si fn devuelve error se hace rollback.
// Llamar a InTx dentro de fn reutiliza la transacción en curso.
//
// Reglas de uso:
//   - Cualquier error que devuelva fn hace rollback de todo lo que hizo. Lo que
//     debe quedar aunque la operación falle (ej. el contador de intentos
//     fallidos de un login) se guarda en su propio InTx, y el error de dominio
//     se devuelve fuera de él.
//   - Dentro de fn se usa solo el ctx que recibe fn. Con el ctx de afuera las
//     consultas no ven la transacción y una escritura espera su lock.
//   - Un InTx anidado no tiene savepoints: si el interno devuelve un error y el
//     externo lo ignora, lo que escribió el interno se confirma igual.
//   - El ctx de fn, y todo ctx derivado de él (también con
//     context.WithoutCancel), sigue apuntando a la transacción después de que
//     termine: lo que se haga con él falla. Una goroutine que trabaje con la
//     base arranca con un ctx derivado de afuera de la transacción (ej.
//     context.WithoutCancel(ctxExterno)), o recién después de que InTx vuelva;
//     nunca con el ctx de fn. Así nada se confirma solo, fuera de la transacción.
//   - La implementación nunca limita el pool a una conexión (en SQLite,
//     nunca SetMaxOpenConns(1)): una consulta con un ctx sin la transacción
//     esperaría para siempre una conexión libre.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}
