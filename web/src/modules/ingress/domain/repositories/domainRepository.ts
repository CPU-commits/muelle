import type { AppDomain, DomainChanges, NewDomain } from '../entities/appDomain'
import type { AppRef } from '../value-objects/appRef'
import type { Hostname } from '../value-objects/hostname'

// El PUERTO del módulo: los dominios de una app (Fase 3, spec §8, §9 y §10). Contra un
// backend sin la Fase 3, todo rechaza con `UnavailableError` (`phase: 3`).
export interface DomainRepository {
	list(app: AppRef, signal?: AbortSignal): Promise<readonly AppDomain[]>
	/** `HostnameTaken` si el hostname ya existe en muelle. */
	create(app: AppRef, domain: NewDomain): Promise<AppDomain>
	update(app: AppRef, hostname: Hostname, changes: DomainChanges): Promise<AppDomain>
	remove(app: AppRef, hostname: Hostname): Promise<void>
	/** Toma el registro que ya existía en Cloudflare (§9). `NothingToAdopt` si no hay. */
	adopt(app: AppRef, hostname: Hostname): Promise<AppDomain>
	/** Reintenta el DNS y la verificación del certificado. */
	retry(app: AppRef, hostname: Hostname): Promise<AppDomain>
}

export interface IngressDeps {
	readonly domains: DomainRepository
}
