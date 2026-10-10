import {
	ConflictError,
	ForbiddenError,
	NotFoundError,
	RateLimitedError,
	UnauthorizedError,
	ValidationError,
} from '@/shared/domain'

// Un error por cada `error.code` de identity que el panel distingue (mapper_errors.go y
// cmd/http). Los mensajes son para logs: la pantalla elige su texto traducido por clase.

/** `invalid_credentials`: usuario o contraseña incorrectos (no dice cuál). */
export class InvalidCredentials extends UnauthorizedError {
	constructor(options?: ErrorOptions) {
		super('Usuario o contraseña incorrectos', options)
		this.name = 'InvalidCredentials'
	}
}

/** `locked`: el usuario quedó bloqueado un rato por contraseñas o códigos malos. */
export class AccountLocked extends RateLimitedError {
	constructor(options?: ErrorOptions) {
		super('Usuario bloqueado temporalmente', options)
		this.name = 'AccountLocked'
	}
}

/** `rate_limited`: demasiados intentos desde esta IP. */
export class TooManyAttempts extends RateLimitedError {
	constructor(options?: ErrorOptions) {
		super('Demasiados intentos, espera unos minutos', options)
		this.name = 'TooManyAttempts'
	}
}

/** `invalid_code`: el código de 2FA no es el correcto (o ya se usó). */
export class InvalidCode extends UnauthorizedError {
	constructor(options?: ErrorOptions) {
		super('Código incorrecto', options)
		this.name = 'InvalidCode'
	}
}

/** `mfa_required`: la sesión existe pero espera el código de 2FA. */
export class MfaRequired extends UnauthorizedError {
	constructor(options?: ErrorOptions) {
		super('Falta el código de 2FA', options)
		this.name = 'MfaRequired'
	}
}

/** `unauthenticated`: no hay sesión, venció o la sesión pendiente se agotó. */
export class SessionExpired extends UnauthorizedError {
	constructor(options?: ErrorOptions) {
		super('No hay una sesión activa', options)
		this.name = 'SessionExpired'
	}
}

/** `totp_setup_required`: el panel exige 2FA y este usuario todavía no la configuró. */
export class TotpSetupRequired extends ForbiddenError {
	constructor(options?: ErrorOptions) {
		super('Hay que configurar 2FA para continuar', options)
		this.name = 'TotpSetupRequired'
	}
}

/** `invalid_invite`: el link de setup o de invitación no existe, ya se usó o venció. */
export class InvalidInvite extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('La invitación no existe, ya se usó o venció', options)
		this.name = 'InvalidInvite'
	}
}

/** `already_set_up`: el setup inicial ya se hizo; las cuentas nuevas van por invitación. */
export class AlreadySetUp extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('muelle ya tiene usuarios', options)
		this.name = 'AlreadySetUp'
	}
}

/** `conflict` al crear una cuenta: el nombre está tomado. */
export class UsernameTaken extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('Ese usuario ya existe', options)
		this.name = 'UsernameTaken'
	}
}

/** `no_pending_totp`: se pidió activar 2FA sin un secreto pendiente (o se reemplazó). */
export class NoPendingTotp extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('Primero hay que generar el código de 2FA', options)
		this.name = 'NoPendingTotp'
	}
}

/** `no_mfa_pending`: se mandó un código con una sesión que ya está completa. */
export class NoMfaPending extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('Esta sesión no espera un código', options)
		this.name = 'NoMfaPending'
	}
}

export class InvalidUsername extends ValidationError {
	constructor() {
		super('Entre 3 y 32 caracteres: a-z, 0-9, ".", "_" o "-"', 'username')
		this.name = 'InvalidUsername'
	}
}

export type PasswordProblem = 'empty' | 'short' | 'long'

export class InvalidPassword extends ValidationError {
	readonly problem: PasswordProblem

	constructor(problem: PasswordProblem) {
		super(`Contraseña inválida (${problem})`, 'password')
		this.name = 'InvalidPassword'
		this.problem = problem
	}
}

/** El código no tiene 6 dígitos: ni se manda. */
export class MalformedCode extends ValidationError {
	constructor() {
		super('El código tiene 6 dígitos', 'code')
		this.name = 'MalformedCode'
	}
}

/** El link llegó sin token (`/setup` sin `?token=`). */
export class MissingInviteToken extends ValidationError {
	constructor() {
		super('Falta el token del link', 'token')
		this.name = 'MissingInviteToken'
	}
}
