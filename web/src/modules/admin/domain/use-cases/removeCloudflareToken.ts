import type { CloudflareStatus, Settings } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'

/**
 * Sin token, muelle deja de crear registros DNS y pide los certificados por HTTP. `''` lo
 * borra junto con las zonas que veía (contrato §4.1). Con los certificados de Cloudflare
 * encendidos (`current.originCa`), los apaga en el mismo cambio: sin token no se pueden
 * usar, y el backend rechaza quitarlo sin `cloudflare_origin_ca: false`.
 */
export type RemoveCloudflareToken = (current: CloudflareStatus) => Promise<Settings>

export function makeRemoveCloudflareToken({ settings }: SettingsDeps): RemoveCloudflareToken {
	return async (current) =>
		settings.patch({
			cloudflareToken: '',
			...(current.originCa ? { cloudflareOriginCa: false } : {}),
		})
}
