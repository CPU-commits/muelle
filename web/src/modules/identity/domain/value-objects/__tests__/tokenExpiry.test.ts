import { describe, expect, it } from 'vitest'
import {
	expiresInDays,
	expiryInputRange,
	InvalidTokenExpiry,
	TOKEN_MAX_DAYS,
	tokenExpiryProblem,
} from '../..'

// Medianoche y casi medianoche locales: el resultado no depende de la hora del día.
const TODAY = new Date(2026, 9, 10, 23, 45)

describe('tokenExpiry', () => {
	it('sin fecha no vence (0 días)', () => {
		expect(expiresInDays('', TODAY)).toBe(0)
		expect(tokenExpiryProblem('  ', TODAY)).toBeNull()
	})

	it('cuenta los días desde hoy hasta la fecha elegida', () => {
		expect(expiresInDays('2026-10-11', TODAY)).toBe(1)
		expect(expiresInDays('2026-12-31', TODAY)).toBe(82)
		// Cruza el cambio de horario de muchos países sin perder un día.
		expect(expiresInDays('2027-04-10', TODAY)).toBe(182)
	})

	it('rechaza hoy, el pasado y más de 3650 días', () => {
		expect(tokenExpiryProblem('2026-10-10', TODAY)).toBe('past')
		expect(tokenExpiryProblem('2025-01-01', TODAY)).toBe('past')
		// 2026-10-10 + 3650 días (con tres 29 de febrero en el medio).
		expect(tokenExpiryProblem('2036-10-07', TODAY)).toBeNull()
		expect(tokenExpiryProblem('2036-10-08', TODAY)).toBe('too_far')
		expect(() => expiresInDays('2025-01-01', TODAY)).toThrow(InvalidTokenExpiry)
	})

	it('rechaza lo que no es una fecha que existe', () => {
		expect(tokenExpiryProblem('31/12/2026', TODAY)).toBe('invalid')
		expect(tokenExpiryProblem('2027-02-30', TODAY)).toBe('invalid')
	})

	it('da los límites del input: de mañana a 3650 días', () => {
		const range = expiryInputRange(TODAY)
		expect(range.min).toBe('2026-10-11')
		expect(expiresInDays(range.max, TODAY)).toBe(TOKEN_MAX_DAYS)
	})
})
