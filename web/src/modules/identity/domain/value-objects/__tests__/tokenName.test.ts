import { describe, expect, it } from 'vitest'
import { InvalidTokenName, parseTokenName, TOKEN_NAME_MAX_LENGTH, tokenNameProblem } from '../..'

describe('tokenName', () => {
	it('acepta un nombre normal y le saca los espacios de los bordes', () => {
		expect(parseTokenName('  github-actions ')).toBe('github-actions')
		expect(parseTokenName('CI de auditfy ✓')).toBe('CI de auditfy ✓')
	})

	it('es obligatorio', () => {
		expect(tokenNameProblem('')).toBe('empty')
		expect(tokenNameProblem('   ')).toBe('empty')
		expect(() => parseTokenName(' ')).toThrow(InvalidTokenName)
	})

	it('cuenta caracteres, no bytes, hasta 64', () => {
		expect(tokenNameProblem('ñ'.repeat(TOKEN_NAME_MAX_LENGTH))).toBeNull()
		expect(tokenNameProblem('a'.repeat(TOKEN_NAME_MAX_LENGTH + 1))).toBe('long')
	})

	it('rechaza caracteres de control e invisibles, como el backend', () => {
		expect(tokenNameProblem('ci\nprod')).toBe('invisible')
		expect(tokenNameProblem('ci​prod')).toBe('invisible')
		const error = (() => {
			try {
				parseTokenName('a\tb')
			} catch (caught) {
				return caught
			}
			return null
		})()
		expect(error).toBeInstanceOf(InvalidTokenName)
		expect((error as InvalidTokenName).field).toBe('name')
		expect((error as InvalidTokenName).problem).toBe('invisible')
	})
})
