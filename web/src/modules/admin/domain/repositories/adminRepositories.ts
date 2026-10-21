import type { AuditEvent, AuditEventId } from '../entities/auditEvent'
import type { NewRegistry, Registry, RegistryCredentials } from '../entities/registry'
import type { Settings, SettingsPatch } from '../entities/settings'

// Los puertos de admin. La implementación HTTP traduce los `error.code` del backend a
// los errores de `errors/adminErrors.ts`; ningún método devuelve un status.

export interface SettingsRepository {
	get(signal?: AbortSignal): Promise<Settings>
	/** Aplica los campos presentes y devuelve los ajustes resultantes. */
	patch(patch: SettingsPatch): Promise<Settings>
}

export interface AuditQuery {
	readonly limit: number
	/** Solo los eventos con id menor; sin él, desde el más nuevo. */
	readonly before?: AuditEventId
}

export interface AuditRepository {
	list(query: AuditQuery, signal?: AbortSignal): Promise<AuditEvent[]>
}

/** `/registries` (contrato §2.10): las credenciales de los registries privados. */
export interface RegistryRepository {
	list(signal?: AbortSignal): Promise<Registry[]>
	create(registry: NewRegistry): Promise<Registry>
	update(host: string, credentials: RegistryCredentials): Promise<Registry>
	remove(host: string): Promise<void>
}

export interface SettingsDeps {
	readonly settings: SettingsRepository
}

export interface AuditDeps {
	readonly audit: AuditRepository
}

export interface RegistryDeps {
	readonly registries: RegistryRepository
}
