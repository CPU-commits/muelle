import { type Password, parseLoginPassword, parseNewPassword } from './password'
import { parseLoginUsername, parseUsername, type Username } from './username'

export interface CredentialsInput {
	username: string
	password: string
}

export interface LoginCredentials {
	readonly username: Username
	readonly password: Password
}

/** El usuario y la contraseña de una cuenta nueva (setup o invitación). */
export interface NewAccount {
	readonly username: Username
	readonly password: Password
}

export function parseLoginCredentials(input: CredentialsInput): LoginCredentials {
	return {
		username: parseLoginUsername(input.username),
		password: parseLoginPassword(input.password),
	}
}

export function parseNewAccount(input: CredentialsInput): NewAccount {
	return {
		username: parseUsername(input.username),
		password: parseNewPassword(input.password),
	}
}
