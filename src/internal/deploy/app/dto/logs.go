package dto

import "time"

// AppLogsRequest es GET …/logs (contrato §2.9) con la query tal cual: el caso
// de uso la valida después de buscar la app (el 404 va antes que el 422).
// Tail, Since y Follow son nil si no vienen.
type AppLogsRequest struct {
	AppRef
	Service string
	Tail    *string
	Since   *string
	Follow  *string
}

// AppLogLine es una línea del log de un servicio (el data de un evento log,
// contrato §2.9), ya unida, enmascarada y cortada. Time y Task son nil en un
// pedazo de una línea larga que llegó sin la línea a la que pertenece.
type AppLogLine struct {
	Time   *time.Time // truncada al milisegundo
	Task   *string    // <servicio>.<slot> o <servicio>.<7 del id>
	Stream string
	Line   string
}
