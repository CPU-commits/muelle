import { describe, expect, it } from 'vitest'
import { MalformedCode, normalizeTotpCode, parseTotpCode } from '../..'

describe('totpCode', () => {
	it('acepta 6 dígitos, también con los espacios que muestran las apps', () => {
		expect(parseTotpCode('123456')).toBe('123456')
		expect(parseTotpCode(' 123 456 ')).toBe('123456')
		expect(normalizeTotpCode('12 34')).toBe('1234')
	})

	it('rechaza cualquier otra cosa', () => {
		expect(() => parseTotpCode('12345')).toThrow(MalformedCode)
		expect(() => parseTotpCode('1234567')).toThrow(MalformedCode)
		expect(() => parseTotpCode('12345a')).toThrow(MalformedCode)
	})
})
