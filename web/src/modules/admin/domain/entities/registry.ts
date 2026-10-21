import type { RegistryHost } from '../value-objects/registryHost'

/** Un registry privado (spec §5.1). La contraseña se guarda cifrada y no vuelve nunca. */
export interface Registry {
	readonly host: string
	readonly username: string
	readonly createdAt: Date
	readonly updatedAt: Date
}

export interface NewRegistry {
	readonly host: RegistryHost
	readonly username: string
	readonly password: string
}

/** Reemplazar las credenciales: la contraseña se escribe de nuevo, siempre. */
export interface RegistryCredentials {
	readonly username: string
	readonly password: string
}

/** La fase en la que llega `/registries` (los deploys la usan para imágenes privadas). */
export const REGISTRIES_PHASE = 2 as const

/**
 * El registry de las imágenes de muelle (`ghcr.io/cpu-commits/muelle…`, contrato §5.1). Son
 * privadas: install guarda esta credencial en el primer arranque, y con ella ingress baja
 * la imagen de Caddy de cada versión nueva (§3.4 y §5.2).
 */
export const MUELLE_IMAGES_HOST = 'ghcr.io'

/** Borrarlo deja a muelle sin poder bajar sus propias imágenes. */
export function isMuelleRegistry(host: string): boolean {
	return host === MUELLE_IMAGES_HOST
}
