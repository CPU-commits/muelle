import { z } from 'zod'
import { http } from '@/shared/data/http/client'
import { parseOrThrow } from '@/shared/data/http/parse'
import {
	type AuditEventDto,
	auditEventDtoSchema,
	type NewRegistryPayload,
	type RegistryCredentialsPayload,
	type RegistryDto,
	registryDtoSchema,
	type SettingsDto,
	settingsDtoSchema,
	type SettingsPatchPayload,
} from '../dtos/adminDto'

const auditSchema = z.array(auditEventDtoSchema)
const registriesSchema = z.array(registryDtoSchema)

/**
 * El `PATCH /settings` responde cuando el backend aplicó el cambio del proxy (DNS, la
 * configuración de Caddy, agendar su recreación), con su propio plazo de 2 min (contrato
 * §3.5): esperar 150 s y no los 30 s de siempre.
 */
export const SETTINGS_PATCH_TIMEOUT_MS = 150_000

function withSignal(signal: AbortSignal | undefined) {
	return signal ? { signal } : {}
}

export const adminHttpDataSource = {
	async getSettings(signal?: AbortSignal): Promise<SettingsDto> {
		const payload = await http.get<unknown>('/settings', withSignal(signal))
		return parseOrThrow(settingsDtoSchema, payload, 'admin.settings')
	},

	async patchSettings(body: SettingsPatchPayload): Promise<SettingsDto> {
		const payload = await http.patch<unknown>('/settings', {
			body,
			timeoutMs: SETTINGS_PATCH_TIMEOUT_MS,
		})
		return parseOrThrow(settingsDtoSchema, payload, 'admin.settings.patch')
	},

	async listAudit(
		query: { limit: number; before?: number },
		signal?: AbortSignal,
	): Promise<AuditEventDto[]> {
		const payload = await http.get<unknown>('/audit', { query, ...withSignal(signal) })
		return parseOrThrow(auditSchema, payload, 'admin.audit')
	},

	// `/registries` (contrato §2.10), con el host url-encoded en la ruta.

	async listRegistries(signal?: AbortSignal): Promise<RegistryDto[]> {
		const payload = await http.get<unknown>('/registries', withSignal(signal))
		return parseOrThrow(registriesSchema, payload, 'admin.registries')
	},

	async createRegistry(body: NewRegistryPayload): Promise<RegistryDto> {
		const payload = await http.post<unknown>('/registries', { body })
		return parseOrThrow(registryDtoSchema, payload, 'admin.registries.create')
	},

	async updateRegistry(host: string, body: RegistryCredentialsPayload): Promise<RegistryDto> {
		const payload = await http.patch<unknown>(`/registries/${encodeURIComponent(host)}`, {
			body,
		})
		return parseOrThrow(registryDtoSchema, payload, 'admin.registries.update')
	},

	async deleteRegistry(host: string): Promise<void> {
		await http.delete<unknown>(`/registries/${encodeURIComponent(host)}`)
	},
}
