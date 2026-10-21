import { ConflictError, NotFoundError, ValidationError } from '@/shared/domain'

// Un error por cada `error.code` de identity que el panel distingue (mapper_errors.go y
// cmd/http), más los de validación que el dominio detecta antes de mandar nada. Los
// mensajes son para logs: la pantalla elige su texto traducido por clase.

/** `cannot_delete_self`: nadie puede borrar su propio usuario. */
export class CannotDeleteSelf extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('No puedes borrar tu propio usuario', options)
		this.name = 'CannotDeleteSelf'
	}
}

/** 404 en `DELETE /users/{u}` o `DELETE /users/{u}/totp`: lo borró otra persona. */
export class UserNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('El usuario ya no existe', options)
		this.name = 'UserNotFound'
	}
}

/** 404 en `DELETE /tokens/{id}`: ya estaba revocado. */
export class TokenNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('El token ya no existe', options)
		this.name = 'TokenNotFound'
	}
}

/** 404 en `POST /tokens`: el proyecto o el stage del alcance no existe (o se borró). */
export class TokenScopeNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('El proyecto o el stage no existe', options)
		this.name = 'TokenScopeNotFound'
	}
}

export type TokenNameProblem = 'empty' | 'long' | 'invisible'

export class InvalidTokenName extends ValidationError {
	readonly problem: TokenNameProblem

	constructor(problem: TokenNameProblem) {
		super(`Nombre de token inválido (${problem})`, 'name')
		this.name = 'InvalidTokenName'
		this.problem = problem
	}
}

/** Todo token tiene un proyecto: sin él no hay alcance. */
export class MissingTokenProject extends ValidationError {
	constructor() {
		super('Elige un proyecto', 'project')
		this.name = 'MissingTokenProject'
	}
}

export type TokenExpiryProblem = 'invalid' | 'past' | 'too_far'

export class InvalidTokenExpiry extends ValidationError {
	readonly problem: TokenExpiryProblem

	constructor(problem: TokenExpiryProblem) {
		super(`Vencimiento inválido (${problem})`, 'expiresOn')
		this.name = 'InvalidTokenExpiry'
		this.problem = problem
	}
}
