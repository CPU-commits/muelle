import type { AppDomain } from '../entities/appDomain'
import type { IngressDeps } from '../repositories/domainRepository'
import type { DomainRef } from './removeDomain'

export type AdoptRecord = (ref: DomainRef) => Promise<AppDomain>

/** "Adoptar registro": apunta el registro existente a la IP pública y lo maneja (§9). */
export function makeAdoptRecord({ domains }: IngressDeps): AdoptRecord {
	return async ({ app, hostname }) => domains.adopt(app, hostname)
}
