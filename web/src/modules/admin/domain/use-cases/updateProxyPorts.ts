import type { Settings } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'
import { parseProxyPorts } from '../value-objects/port'

export interface UpdateProxyPortsInput {
	http: string
	https: string
}

/** Cambiarlos recrea muelle-caddy: los sitios se caen unos segundos. */
export type UpdateProxyPorts = (input: UpdateProxyPortsInput) => Promise<Settings>

export function makeUpdateProxyPorts({ settings }: SettingsDeps): UpdateProxyPorts {
	return async ({ http, https }) => {
		const ports = parseProxyPorts(http, https)
		return settings.patch({ httpPort: ports.http, httpsPort: ports.https })
	}
}
