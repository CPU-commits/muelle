import type { RegistryCredentials } from '../entities/registry'
import { RequiredField } from '../errors/adminErrors'

export function parseRegistryCredentials(input: {
	username: string
	password: string
}): RegistryCredentials {
	const username = input.username.trim()
	if (username === '') throw new RequiredField('username')
	// La contraseña va tal cual: un espacio puede ser parte de ella.
	if (input.password === '') throw new RequiredField('password')
	return { username, password: input.password }
}
