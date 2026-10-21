import type { Settings } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'
import { parseCloudflareToken } from '../value-objects/cloudflareToken'

/**
 * El token se guarda cifrado y no vuelve: después solo se ve si está y cuántas zonas ve.
 * El backend se lo muestra a Cloudflare antes de guardarlo: si lo rechaza o no ve zonas es
 * un 422 en el campo; si Cloudflare no responde, `CloudflareUnavailable`.
 */
export type ReplaceCloudflareToken = (token: string) => Promise<Settings>

export function makeReplaceCloudflareToken({ settings }: SettingsDeps): ReplaceCloudflareToken {
	return async (token) => settings.patch({ cloudflareToken: parseCloudflareToken(token) })
}
