import type { AppDomain } from '../entities/appDomain'
import type { IngressDeps } from '../repositories/domainRepository'
import type { AppRef } from '../value-objects/appRef'
import { parseTarget } from '../value-objects/domainTarget'
import type { Hostname } from '../value-objects/hostname'

export interface ChangeTargetInput {
	readonly app: AppRef
	readonly hostname: Hostname
	/** "servicio:puerto" */
	readonly target: string
}

export type ChangeTarget = (input: ChangeTargetInput) => Promise<AppDomain>

/** "Editar": cambia a qué servicio y puerto apunta la ruta de Caddy. */
export function makeChangeTarget({ domains }: IngressDeps): ChangeTarget {
	return async ({ app, hostname, target }) =>
		domains.update(app, hostname, { target: parseTarget(target) })
}
