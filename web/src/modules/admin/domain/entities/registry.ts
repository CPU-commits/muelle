import type { RegistryHost } from '../value-objects/registryHost'

/** Un registry privado (spec §5.1). La contraseña se guarda cifrada y no vuelve nunca. */
export interface Registry {
	readonly host: string
	readonly username: string
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
