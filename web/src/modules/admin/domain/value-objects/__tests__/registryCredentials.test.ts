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
// control ni ":", contraseña de 1 a 4096 bytes sin saltos de línea.
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

	it('el usuario no lleva ":", que la autenticación HTTP Basic usa de separador', () => {
		expect(problemOf('bot:ci', 'x')).toBe('username:colon')
		// En la contraseña sí puede ir.
		expect(problemOf('bot', 'ghp:x')).toBe('ok')
	})

	it('el largo del usuario se cuenta en caracteres (runas), no en unidades UTF-16', () => {
		// Un emoji es una runa para el backend (utf8.RuneCountInString) y dos para .length.
		expect(problemOf('😀'.repeat(255), 'x')).toBe('ok')
		expect(problemOf('ñ'.repeat(255), 'x')).toBe('ok')
		expect(problemOf('😀'.repeat(256), 'x')).toBe('username:too_long')
	})

	it('caracteres de control son los de unicode.IsControl: C0, DEL y C1; U+2028 no lo es', () => {
		// Adentro: en los bordes, el usuario se recorta como cualquier campo de texto.
		for (const control of ['\u0000', '\n', '\u007f', '\u0085', '\u009f']) {
			expect(problemOf(`bo${control}t`, 'x')).toBe('username:control')
		}
		// Separadores de línea y de párrafo Unicode: el backend los acepta en el usuario.
		expect(problemOf('bot\u2028ci', 'x')).toBe('ok')
		expect(problemOf('bot\u2029ci', 'x')).toBe('ok')
	})

	it('en la contraseña solo \\n y \\r son saltos de línea: un tab o U+2028 pasan', () => {
		expect(problemOf('bot', 'con\ttab')).toBe('ok')
		expect(problemOf('bot', 'ghp\u2028x')).toBe('ok')
		expect(problemOf('bot', 'ghp\u0085x')).toBe('ok')
		// 1024 emoji son 4096 bytes en UTF-8; uno más se pasa.
		expect(problemOf('bot', '😀'.repeat(1024))).toBe('ok')
		expect(problemOf('bot', '😀'.repeat(1025))).toBe('password:too_long')
		expect(problemOf('bot', 'ghp_x\r\n')).toBe('password:newline')
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
