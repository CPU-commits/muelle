import { describe, expect, it } from 'vitest'
import {
	InvalidRegistryCredential,
	parseRegistryCredentials,
	type RegistryCredentialProblem,
	REGISTRY_PASSWORD_MAX_BYTES,
	REGISTRY_USERNAME_MAX_LENGTH,
	RequiredField,
} from '../..'

function problemOf(username: string, password: string): string {
	try {
		parseRegistryCredentials({ username, password })
		return 'ok'
	} catch (error) {
		if (error instanceof InvalidRegistryCredential) return `${error.field}:${error.problem}`
		return error instanceof RequiredField ? `${error.field ?? ''}:required` : 'other'
	}
}

const problems = (...list: `${'username' | 'password'}:${RegistryCredentialProblem}`[]) => list

// Los límites del backend (contrato §2.10): usuario de 1 a 255 caracteres sin caracteres de
// control, contraseña de 1 a 4096 bytes sin saltos de línea.
describe('registryCredentials', () => {
	it('usuario y contraseña son obligatorios; la contraseña no se recorta', () => {
		expect(parseRegistryCredentials({ username: ' bot ', password: ' x ' })).toEqual({
			username: 'bot',
			password: ' x ',
		})
		expect(problemOf(' ', 'x')).toBe('username:required')
		expect(problemOf('bot', '')).toBe('password:required')
	})

	it('el usuario tiene hasta 255 caracteres y ninguno de control', () => {
		expect(REGISTRY_USERNAME_MAX_LENGTH).toBe(255)
		expect(problemOf('a'.repeat(255), 'x')).toBe('ok')
		expect([problemOf('a'.repeat(256), 'x'), problemOf('bo\tt', 'x')]).toEqual(
			problems('username:too_long', 'username:control'),
		)
	})

	it('la contraseña tiene hasta 4096 bytes (no caracteres) y ningún salto de línea', () => {
		expect(REGISTRY_PASSWORD_MAX_BYTES).toBe(4096)
		expect(problemOf('bot', 'x'.repeat(4096))).toBe('ok')
		// "ñ" son dos bytes en UTF-8: 2049 son 4098 bytes.
		expect([
			problemOf('bot', 'x'.repeat(4097)),
			problemOf('bot', 'ñ'.repeat(2049)),
			problemOf('bot', 'ghp_x\n'),
			problemOf('bot', 'ghp\r_x'),
		]).toEqual(
			problems(
				'password:too_long',
				'password:too_long',
				'password:newline',
				'password:newline',
			),
		)
	})
})
