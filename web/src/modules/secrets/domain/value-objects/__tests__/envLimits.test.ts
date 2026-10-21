import { describe, expect, it } from 'vitest'
import { envValueProblem, envVarSize, MAX_VALUE_BYTES, utf8Length } from '../envLimits'

describe('envLimits', () => {
	it('mide en bytes UTF-8, como el backend', () => {
		const samples = ['', 'abc', 'ñandú', '€', '😀', 'línea\nnueva']
		for (const text of samples) {
			expect(utf8Length(text), text).toBe(new TextEncoder().encode(text).length)
		}
		expect(envVarSize('KEY', 'ñ')).toBe(5)
	})

	it('un surrogate suelto cuenta como el carácter de reemplazo', () => {
		expect(utf8Length('\uD800')).toBe(3)
	})

	it('el valor puede tener hasta 64 KiB en bytes, no en caracteres', () => {
		expect(envValueProblem('a'.repeat(MAX_VALUE_BYTES))).toBeNull()
		expect(envValueProblem('a'.repeat(MAX_VALUE_BYTES + 1))).toBe('too_large')
		// 2 bytes por carácter: la mitad de caracteres ya llega al límite.
		expect(envValueProblem('ñ'.repeat(MAX_VALUE_BYTES / 2))).toBeNull()
		expect(envValueProblem('ñ'.repeat(MAX_VALUE_BYTES / 2 + 1))).toBe('too_large')
	})

	it('rechaza el UTF-8 inválido y el NUL; acepta vacío y varias líneas', () => {
		expect(envValueProblem('a\uDC00b')).toBe('invalid_utf8')
		expect(envValueProblem('a\0b')).toBe('nul')
		expect(envValueProblem('')).toBeNull()
		expect(envValueProblem('-----BEGIN-----\nabc\n-----END-----')).toBeNull()
	})
})
