import type { AppDomain } from '../entities/appDomain'
import type { IngressDeps } from '../repositories/domainRepository'
import type { DomainRef } from './removeDomain'

export type RetryDomain = (ref: DomainRef) => Promise<AppDomain>

/** "Reintentar" / "Verificar": vuelve a intentar el DNS y el certificado (§8, §9). */
export function makeRetryDomain({ domains }: IngressDeps): RetryDomain {
	return async ({ app, hostname }) => domains.retry(app, hostname)
}
