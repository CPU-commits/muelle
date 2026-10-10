import type { Settings } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'
import { parsePublicIpv4, parsePublicIpv6 } from '../value-objects/ipAddress'

export interface UpdatePublicIpInput {
	ipv4: string
	/** `''`: sin IPv6. */
	ipv6: string
}

/** A dónde apuntan los registros DNS que crea muelle; cambiarla los actualiza. */
export type UpdatePublicIp = (input: UpdatePublicIpInput) => Promise<Settings>

export function makeUpdatePublicIp({ settings }: SettingsDeps): UpdatePublicIp {
	return async ({ ipv4, ipv6 }) =>
		settings.patch({ publicIpv4: parsePublicIpv4(ipv4), publicIpv6: parsePublicIpv6(ipv6) })
}
