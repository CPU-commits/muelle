import { describe, expect, it } from 'vitest'
import { MissingInviteToken, parseInviteToken } from '../..'

describe('inviteToken', () => {
	it('acepta el token tal cual, sin espacios alrededor', () => {
		expect(parseInviteToken(' abc_DEF-123 ')).toBe('abc_DEF-123')
	})

	it('sin token no hay nada que mandar', () => {
		expect(() => parseInviteToken(null)).toThrow(MissingInviteToken)
		expect(() => parseInviteToken(undefined)).toThrow(MissingInviteToken)
		expect(() => parseInviteToken('  ')).toThrow(MissingInviteToken)
	})
})
