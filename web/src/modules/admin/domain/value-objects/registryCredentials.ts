import type { RegistryCredentials } from '../entities/registry'
import { InvalidRegistryCredential, RequiredField } from '../errors/adminErrors'

/** Los límites del backend (contrato §2.10). */
export const REGISTRY_USERNAME_MAX_LENGTH = 255
export const REGISTRY_PASSWORD_MAX_BYTES = 4096

const CONTROL = /\p{Cc}/u
const NEWLINE = /[\r\n]/

export function parseRegistryCredentials(input: {
	username: string
	password: string
}): RegistryCredentials {
	const username = input.username.trim()
	if (username === '') throw new RequiredField('username')
	if ([...username].length > REGISTRY_USERNAME_MAX_LENGTH) {
		throw new InvalidRegistryCredential('username', 'too_long')
	}
	if (CONTROL.test(username)) throw new InvalidRegistryCredential('username', 'control')
	// La contraseña va tal cual: un espacio puede ser parte de ella.
	const { password } = input
	if (password === '') throw new RequiredField('password')
	if (new TextEncoder().encode(password).length > REGISTRY_PASSWORD_MAX_BYTES) {
		throw new InvalidRegistryCredential('password', 'too_long')
	}
	if (NEWLINE.test(password)) throw new InvalidRegistryCredential('password', 'newline')
	return { username, password }
}
