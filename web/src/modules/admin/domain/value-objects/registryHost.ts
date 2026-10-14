import type { Brand } from '@/shared/domain'
import { InvalidRegistryHost } from '../errors/adminErrors'

export type RegistryHost = Brand<string, 'RegistryHost'>

// Lo que va antes de la primera "/" de una imagen: `ghcr.io`, `registry.example.com:5000`,
// `localhost:5000`. Sin esquema ni ruta.
const HOST = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:\d{1,5})?$/

/** Acepta lo que se pega desde el navegador (`https://ghcr.io/`) y lo deja como host. */
export function normalizeRegistryHost(raw: string): string {
	return raw
		.trim()
		.toLowerCase()
		.replace(/^https?:\/\//, '')
		.replace(/\/+$/, '')
}

export function parseRegistryHost(raw: string): RegistryHost {
	const host = normalizeRegistryHost(raw)
	if (!HOST.test(host)) throw new InvalidRegistryHost()
	return host as RegistryHost
}
