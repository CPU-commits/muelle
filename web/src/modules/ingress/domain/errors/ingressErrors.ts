import { ConflictError, NotFoundError } from '@/shared/domain'

/** El hostname ya es de otro dominio (es único en todo muelle, spec §5.1). */
export class HostnameTaken extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('Ese hostname ya está en uso', options)
		this.name = 'HostnameTaken'
	}
}

/** El dominio no existe (lo borró otra persona, o el hostname de la URL es otro). */
export class DomainNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('Ese dominio no existe', options)
		this.name = 'DomainNotFound'
	}
}

/** "Adoptar" sobre un dominio que no está en `error` por un registro existente. */
export class NothingToAdopt extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('No hay un registro que adoptar', options)
		this.name = 'NothingToAdopt'
	}
}
