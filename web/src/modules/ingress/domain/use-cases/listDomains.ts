import { type AppDomain, sortDomains } from '../entities/appDomain'
import type { IngressDeps } from '../repositories/domainRepository'
import type { AppRef } from '../value-objects/appRef'

export type ListDomains = (app: AppRef, signal?: AbortSignal) => Promise<readonly AppDomain[]>

export function makeListDomains({ domains }: IngressDeps): ListDomains {
	return async (app, signal) => sortDomains(await domains.list(app, signal))
}
