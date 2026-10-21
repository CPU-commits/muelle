import type { AppDomain } from '../entities/appDomain'
import type { IngressDeps } from '../repositories/domainRepository'
import type { AppRef } from '../value-objects/appRef'
import type { Hostname } from '../value-objects/hostname'

export interface SetProxiedInput {
	readonly app: AppRef
	readonly hostname: Hostname
	readonly proxied: boolean
}

export type SetProxied = (input: SetProxiedInput) => Promise<AppDomain>

/** El switch de Proxied: actualiza el registro que maneja muelle (spec §9). */
export function makeSetProxied({ domains }: IngressDeps): SetProxied {
	return async ({ app, hostname, proxied }) => domains.update(app, hostname, { proxied })
}
