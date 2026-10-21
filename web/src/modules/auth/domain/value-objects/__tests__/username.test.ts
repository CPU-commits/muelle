import { describe, expect, it } from 'vitest'
import {
	InvalidUsername,
	isValidUsername,
	parseLoginUsername,
	parseUsername,
	usernameInitial,
} from '../..'

describe('username', () => {
	it('normaliza mayúsculas y espacios antes de validar', () => {
		expect(parseUsername('  Felipe ')).toBe('felipe')
		expect(isValidUsername('Ana.Maria')).toBe(true)
	})

	it('aplica la regla del backend para una cuenta nueva', () => {
		expect(parseUsername('ci_bot-2')).toBe('ci_bot-2')
		expect(() => parseUsername('ab')).toThrow(InvalidUsername)
		expect(() => parseUsername('_felipe')).toThrow(InvalidUsername)
		expect(() => parseUsername('a'.repeat(33))).toThrow(InvalidUsername)
		expect(() => parseUsername('josé')).toThrow(InvalidUsername)
	})

	it('para entrar solo exige que no esté vacío', () => {
		expect(parseLoginUsername(' XY ')).toBe('xy')
		expect(() => parseLoginUsername('   ')).toThrow(InvalidUsername)
	})

	it('la inicial va en mayúscula', () => {
		expect(usernameInitial('felipe')).toBe('F')
		expect(usernameInitial('')).toBe('?')
	})
})
