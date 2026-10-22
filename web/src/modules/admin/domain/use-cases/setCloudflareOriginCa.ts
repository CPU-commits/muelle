import type { CloudflareStatus, Settings } from '../entities/settings'
import { OriginCaNeedsToken } from '../errors/adminErrors'
import type { SettingsDeps } from '../repositories/adminRepositories'

/**
 * Enciende o apaga los certificados de Cloudflare (Origin CA, contrato §3.7). Encenderlos
 * necesita el token: sin él es `OriginCaNeedsToken`, sin llamar al backend. Con el token, el
 * backend revisa que tenga el permiso Zone → SSL and Certificates → Edit en sus zonas (un
 * `422` en `cloudflareOriginCa`, con el motivo) y, si Cloudflare no responde,
 * `CloudflareUnavailable`. Apagarlos se puede siempre.
 */
export type SetCloudflareOriginCa = (
	enabled: boolean,
	current: CloudflareStatus,
) => Promise<Settings>

export function makeSetCloudflareOriginCa({ settings }: SettingsDeps): SetCloudflareOriginCa {
	return async (enabled, current) => {
		if (enabled && !current.tokenSet) throw new OriginCaNeedsToken()
		return settings.patch({ cloudflareOriginCa: enabled })
	}
}
