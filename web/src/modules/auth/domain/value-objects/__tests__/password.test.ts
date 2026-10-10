import { describe, expect, it } from 'vitest'
import {
	InvalidPassword,
	PASSWORD_MAX_BYTES,
	PASSWORD_MIN_LENGTH,
	parseLoginPassword,
	parseNewPassword,
	passwordProblem,
} from '../..'

describe('password', () => {
	it('cuenta caracteres, no bytes, para el mínimo', () => {
		expect(passwordProblem('ñ'.repeat(PASSWORD_MIN_LENGTH))).toBeNull()
		expect(passwordProblem('a'.repeat(PASSWORD_MIN_LENGTH - 1))).toBe('short')
		expect(passwordProblem('')).toBe('empty')
	})

	it('cuenta bytes para el máximo', () => {
		expect(passwordProblem('a'.repeat(PASSWORD_MAX_BYTES))).toBeNull()
		// "ñ" son 2 bytes en UTF-8: 129 caracteres pasan de 256 bytes.
		expect(passwordProblem('ñ'.repeat(129))).toBe('long')
	})

	it('una contraseña nueva inválida dice por qué', () => {
		try {
			parseNewPassword('corta')
			expect.unreachable('debió fallar')
		} catch (error) {
			expect(error).toBeInstanceOf(InvalidPassword)
			expect((error as InvalidPassword).problem).toBe('short')
			expect((error as InvalidPassword).field).toBe('password')
		}
	})

	it('nunca recorta la contraseña', () => {
		expect(parseNewPassword('  doce espacios  ')).toBe('  doce espacios  ')
	})

	it('para entrar acepta cualquier largo menos vacía o sobre el máximo', () => {
		expect(parseLoginPassword('x')).toBe('x')
		expect(() => parseLoginPassword('')).toThrow(InvalidPassword)
		expect(() => parseLoginPassword('a'.repeat(PASSWORD_MAX_BYTES + 1))).toThrow(
			InvalidPassword,
		)
	})
})
