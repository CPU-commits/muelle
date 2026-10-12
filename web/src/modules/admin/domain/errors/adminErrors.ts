import { ConflictError, ValidationError } from '@/shared/domain'

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

/** `conflict` al agregar un registry: ya hay uno con ese host. */
export class RegistryExists extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('Ya hay un registry con ese host', options)
		this.name = 'RegistryExists'
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

export class InvalidAcmeEmail extends ValidationError {
	constructor() {
		super('No es un email válido', 'acmeEmail')
		this.name = 'InvalidAcmeEmail'
	}
}

export class InvalidRegistryHost extends ValidationError {
	constructor() {
		super('Un host como ghcr.io o registry.example.com:5000', 'host')
		this.name = 'InvalidRegistryHost'
	}
}
