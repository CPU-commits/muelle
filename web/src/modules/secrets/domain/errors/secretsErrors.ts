import { ForbiddenError, NotFoundError, ValidationError } from '@/shared/domain'

// Los errores de las variables. Los mensajes son para logs: la pantalla elige su texto
// traducido por la clase (y por `problem`).

/** El campo del cuerpo donde viene la variable, como lo nombra el backend en un 422. */
export type EnvField = 'vars' | 'set' | 'unset'

export type EnvKeyProblem = 'empty' | 'format' | 'too_long' | 'reserved'

/** `validate.EnvKey`: letras, números y '_', sin empezar con número, hasta 128, sin `COMPOSE_`. */
export class InvalidEnvKey extends ValidationError {
	readonly problem: EnvKeyProblem
	readonly key: string

	constructor(problem: EnvKeyProblem, key: string, field: EnvField) {
		super(`Clave ${JSON.stringify(key)} inválida (${problem})`, field)
		this.name = 'InvalidEnvKey'
		this.problem = problem
		this.key = key
	}
}

export type EnvValueProblem = 'too_large' | 'invalid_utf8' | 'nul'

/** `entity.NewEnvVar`: hasta 64 KiB, UTF-8 válido y sin el carácter NUL. */
export class InvalidEnvValue extends ValidationError {
	readonly problem: EnvValueProblem
	readonly key: string

	constructor(problem: EnvValueProblem, key: string, field: EnvField) {
		super(`El valor de ${JSON.stringify(key)} es inválido (${problem})`, field)
		this.name = 'InvalidEnvValue'
		this.problem = problem
		this.key = key
	}
}

/** Un PATCH sin nada en `set` ni en `unset`: el backend lo rechaza. */
export class EmptyEnvPatch extends ValidationError {
	constructor() {
		super('Indica al menos una variable en set o unset', 'set')
		this.name = 'EmptyEnvPatch'
	}
}

/** La misma clave en `set` y en `unset` del mismo PATCH. */
export class EnvKeyInSetAndUnset extends ValidationError {
	readonly key: string

	constructor(key: string) {
		super(`${JSON.stringify(key)} no puede estar en set y unset a la vez`, 'unset')
		this.name = 'EnvKeyInSetAndUnset'
		this.key = key
	}
}

/**
 * Los límites de un dueño (`entity.MaxVars`, `MaxOwnerBytes`) o del cuerpo de la petición
 * (`MaxBodyBytes`):
 * - `count`: más de 500 variables (en el dueño, en `set` o en `unset`);
 * - `size`: las claves y los valores del dueño suman más de 1 MiB;
 * - `request`: el cuerpo JSON supera 1 MiB.
 */
export type EnvLimit = 'count' | 'size' | 'request'

export class EnvLimitExceeded extends ValidationError {
	readonly limit: EnvLimit

	constructor(limit: EnvLimit, field: EnvField) {
		super(`Límite de variables superado (${limit})`, field)
		this.name = 'EnvLimitExceeded'
		this.limit = limit
	}
}

/**
 * Un 422 con un motivo que el panel no conoce (una regla nueva del backend). Conserva el
 * campo y el motivo para logs; la pantalla muestra un texto propio, traducido.
 */
export class EnvRejected extends ValidationError {
	readonly reason: string

	constructor(field: EnvField | undefined, reason: string, options?: ErrorOptions) {
		super(`El backend rechazó las variables: ${reason}`, field, options)
		this.name = 'EnvRejected'
		this.reason = reason
	}
}

/** 404 `not_found`: el proyecto, el stage o la app no existen (o los borraron). */
export class EnvOwnerNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('El stage o la app no existen', options)
		this.name = 'EnvOwnerNotFound'
	}
}

/** 403 `out_of_scope`: un token sin acceso a este stage. Con sesión no debería pasar. */
export class EnvOutOfScope extends ForbiddenError {
	constructor(options?: ErrorOptions) {
		super('Sin acceso a estas variables', options)
		this.name = 'EnvOutOfScope'
	}
}
