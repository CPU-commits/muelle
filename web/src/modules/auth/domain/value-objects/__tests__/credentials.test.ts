import { describe, expect, it } from 'vitest'
import { InvalidPassword, InvalidUsername, parseLoginCredentials, parseNewAccount } from '../..'

describe('credentials', () => {
	it('para entrar normaliza el usuario y no valida la regla completa', () => {
		expect(parseLoginCredentials({ username: ' FELIPE ', password: 'x' })).toEqual({
			username: 'felipe',
			password: 'x',
		})
	})

	it('una cuenta nueva exige usuario y contraseña válidos', () => {
		expect(parseNewAccount({ username: 'ana', password: 'una-contraseña-larga' })).toEqual({
			username: 'ana',
			password: 'una-contraseña-larga',
		})
		expect(() => parseNewAccount({ username: 'a', password: 'una-contraseña-larga' })).toThrow(
			InvalidUsername,
		)
		expect(() => parseNewAccount({ username: 'ana', password: 'corta' })).toThrow(
			InvalidPassword,
		)
	})
})
