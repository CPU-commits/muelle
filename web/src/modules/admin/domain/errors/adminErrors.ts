import { ConflictError, NotFoundError, UnavailableError, ValidationError } from '@/shared/domain'

// Los `error.code` de admin que el panel distingue (mapper_errors.go) y los errores de
// validación que el dominio detecta antes de mandar nada. Los mensajes son para logs: la
// pantalla elige su texto traducido por clase.

/**
 * `totp_required_first`: quien activa "Exigir 2FA" sin tener su propio 2FA quedaría
 * encerrado en la pantalla de configuración (spec §10.1).
 */
export class TotpRequiredFirst extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('Configura tu propio 2FA antes de exigirlo a todos', options)
		this.name = 'TotpRequiredFirst'
	}
}

/**
 * `would_lock_out`: la lista de IPs permitidas deja fuera la IP con que muelle ve esta
 * sesión. Se guarda igual mandando `confirm_lockout: true` (contrato §4.1).
 */
export class WouldLockOut extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('Con esa lista, tu IP actual no podría abrir el panel', options)
		this.name = 'WouldLockOut'
	}
}

/**
 * `cloudflare_unavailable` (502): Cloudflare no respondió al verificar el token. No es un
 * problema del token: se puede reintentar.
 */
export class CloudflareUnavailable extends UnavailableError {
	constructor(options?: ErrorOptions) {
		super('Cloudflare no respondió', { ...options, reason: 'server' })
		this.name = 'CloudflareUnavailable'
	}
}

/** `conflict` al agregar un registry: ya hay uno con ese host. */
export class RegistryExists extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('Ya hay un registry con ese host', options)
		this.name = 'RegistryExists'
	}
}

/**
 * `not_found` en `PATCH`/`DELETE /registries/{host}`: lo borró otra persona. (Que la ruta
 * todavía no exista es otra cosa: `no_route`, que sale como `UnavailableError` de Fase 2.)
 */
export class RegistryNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('El registry ya no existe', options)
		this.name = 'RegistryNotFound'
	}
}

/** Un campo obligatorio vacío (token de Cloudflare, usuario de un registry…). */
export class RequiredField extends ValidationError {
	constructor(field: string) {
		super('Es obligatorio', field)
		this.name = 'RequiredField'
	}
}

export class InvalidIpAddress extends ValidationError {
	readonly version: 4 | 6

	constructor(version: 4 | 6, field: string) {
		super(`No es una dirección IPv${version} válida`, field)
		this.name = 'InvalidIpAddress'
		this.version = version
	}
}

/** Una línea de "IPs permitidas" que no es una IP ni un rango CIDR. */
export class InvalidCidr extends ValidationError {
	/** La línea del textarea, desde 1. */
	readonly line: number
	readonly value: string

	constructor(line: number, value: string) {
		super(`Línea ${line}: "${value}" no es una IP ni un rango CIDR`, 'panelAllowedCidrs')
		this.name = 'InvalidCidr'
		this.line = line
		this.value = value
	}
}

export type PortProblem = 'format' | 'same'

export class InvalidPort extends ValidationError {
	readonly problem: PortProblem

	constructor(field: 'httpPort' | 'httpsPort', problem: PortProblem) {
		super(`Puerto inválido (${problem})`, field)
		this.name = 'InvalidPort'
		this.problem = problem
	}
}

/**
 * `placeholder`: lleva `{` o `}`. Caddy expande placeholders en el email del emisor ACME y
 * en el token de Cloudflare: con uno, el backend lo guarda pero deja de mandarle la
 * configuración a Caddy (`GET /proxy` dice `out_of_sync`, contrato §3.4). El panel no lo
 * deja guardar.
 */
export type PlaceholderProblem = 'placeholder'

export class InvalidAcmeEmail extends ValidationError {
	readonly problem: 'format' | PlaceholderProblem

	constructor(problem: 'format' | PlaceholderProblem = 'format') {
		super(problem === 'format' ? 'No es un email válido' : 'El email lleva { o }', 'acmeEmail')
		this.name = 'InvalidAcmeEmail'
		this.problem = problem
	}
}

/**
 * Por qué un host no es canónico: el backend no lo normaliza (contrato §2.10), así que el
 * panel explica qué sobra.
 */
export type RegistryHostProblem = 'empty' | 'scheme' | 'path' | 'format'

export class InvalidRegistryHost extends ValidationError {
	readonly problem: RegistryHostProblem

	constructor(problem: RegistryHostProblem) {
		super(`Host de registry inválido (${problem})`, 'host')
		this.name = 'InvalidRegistryHost'
		this.problem = problem
	}
}

/**
 * Qué límite del backend no cumple un usuario o una contraseña de registry (§2.10).
 * `colon`: un `:` en el usuario (HTTP Basic lo usa de separador).
 */
export type RegistryCredentialProblem = 'too_long' | 'control' | 'colon' | 'newline'

export class InvalidRegistryCredential extends ValidationError {
	override readonly field: 'username' | 'password'
	readonly problem: RegistryCredentialProblem

	constructor(field: 'username' | 'password', problem: RegistryCredentialProblem) {
		super(`Credencial de registry inválida (${field}: ${problem})`, field)
		this.name = 'InvalidRegistryCredential'
		this.field = field
		this.problem = problem
	}
}

export type CloudflareTokenProblem = 'length' | 'spaces' | PlaceholderProblem

/**
 * Un token sin la forma de un token de Cloudflare: de 20 a 512 caracteres, sin espacios. Con
 * `{` o `}`, Caddy no carga la configuración (ver `PlaceholderProblem`).
 */
export class InvalidCloudflareToken extends ValidationError {
	readonly problem: CloudflareTokenProblem

	constructor(problem: CloudflareTokenProblem) {
		super(`Token de Cloudflare inválido (${problem})`, 'cloudflareToken')
		this.name = 'InvalidCloudflareToken'
		this.problem = problem
	}
}
