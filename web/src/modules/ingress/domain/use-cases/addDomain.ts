import type { AppDomain } from '../entities/appDomain'
import type { IngressDeps } from '../repositories/domainRepository'
import type { AppRef } from '../value-objects/appRef'
import { parseTarget } from '../value-objects/domainTarget'
import { parseHostname } from '../value-objects/hostname'

export interface AddDomainInput {
	readonly app: AppRef
	readonly hostname: string
	/** "servicio:puerto" */
	readonly target: string
	/** Detrás del proxy de Cloudflare (solo si la zona está en la cuenta). */
	readonly proxied: boolean
}

export type AddDomain = (input: AddDomainInput) => Promise<AppDomain>

/**
 * "Agregar": crea el registro en Cloudflare (si la zona está) y la ruta en Caddy, sin
 * deploy (spec §9). Un hostname o un destino inválidos son `ValidationError` con
 * `field` = `hostname` o `target`.
 */
export function makeAddDomain({ domains }: IngressDeps): AddDomain {
	return async (input) => {
		const hostname = parseHostname(input.hostname)
		const target = parseTarget(input.target)
		return domains.create(input.app, { hostname, target, proxied: input.proxied })
	}
}
