import type { IngressDeps } from '../repositories/domainRepository'
import type { AppRef } from '../value-objects/appRef'
import type { Hostname } from '../value-objects/hostname'

export interface DomainRef {
	readonly app: AppRef
	readonly hostname: Hostname
}

export type RemoveDomain = (ref: DomainRef) => Promise<void>

/** Borra la ruta y, solo si muelle lo maneja, el registro en Cloudflare (spec §9). */
export function makeRemoveDomain({ domains }: IngressDeps): RemoveDomain {
	return async ({ app, hostname }) => domains.remove(app, hostname)
}
