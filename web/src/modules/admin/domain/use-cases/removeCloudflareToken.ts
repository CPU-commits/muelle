import type { Settings } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'

/**
 * Sin token, muelle deja de crear registros DNS y pide los certificados por HTTP. `''` lo
 * borra junto con las zonas que veía (contrato §4.1).
 */
export type RemoveCloudflareToken = () => Promise<Settings>

export function makeRemoveCloudflareToken({ settings }: SettingsDeps): RemoveCloudflareToken {
	return async () => settings.patch({ cloudflareToken: '' })
}
