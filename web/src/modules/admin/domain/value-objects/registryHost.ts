import type { Brand } from '@/shared/domain'
import { InvalidRegistryHost } from '../errors/adminErrors'

export type RegistryHost = Brand<string, 'RegistryHost'>

/** El host de Docker Hub, como lo da `reference.Domain` para cualquier imagen de ahí. */
export const DOCKER_HUB_HOST = 'docker.io'

// Con estos alias la credencial nunca se usaría: una imagen de Docker Hub siempre da
// `docker.io` (contrato §2.10). El backend los rechaza; el panel los convierte.
const DOCKER_HUB_ALIASES = new Set([
	'index.docker.io',
	'registry-1.docker.io',
	'registry.hub.docker.com',
])

// Lo que va antes de la primera "/" de una imagen, en su forma canónica: etiquetas
// [a-z0-9-] separadas por ".", sin guiones en los bordes, y un ":puerto" opcional.
const LABEL = '[a-z0-9]([a-z0-9-]*[a-z0-9])?'
const HOST = new RegExp(`^${LABEL}(\\.${LABEL})*(:(?<port>\\d{1,5}))?$`)

/**
 * Valida el host en la forma canónica que exige el backend, que no normaliza. Solo arregla
 * lo que no cambia de host (espacios, mayúsculas, los alias de Docker Hub); un esquema o
 * una ruta se rechazan con su motivo.
 */
export function parseRegistryHost(raw: string): RegistryHost {
	const host = raw.trim().toLowerCase()
	if (host === '') throw new InvalidRegistryHost('empty')
	if (/^[a-z][a-z0-9+.-]*:\/\//.test(host)) throw new InvalidRegistryHost('scheme')
	if (host.includes('/')) throw new InvalidRegistryHost('path')
	const match = HOST.exec(host)
	const port = match?.groups?.['port']
	if (!match || (port !== undefined && (Number(port) < 1 || Number(port) > 65535))) {
		throw new InvalidRegistryHost('format')
	}
	return (DOCKER_HUB_ALIASES.has(host) ? DOCKER_HUB_HOST : host) as RegistryHost
}
