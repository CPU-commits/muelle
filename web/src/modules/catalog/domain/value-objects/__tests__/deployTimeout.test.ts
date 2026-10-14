import { describe, expect, it } from 'vitest'
import { InvalidDeployTimeout, isValidDeployTimeout, parseDeployTimeout } from '../..'

describe('deployTimeout', () => {
	it('entre 30 y 3600 segundos enteros, como entity.ValidateDeployTimeout', () => {
		expect(parseDeployTimeout(30)).toBe(30)
		expect(parseDeployTimeout(3600)).toBe(3600)
		for (const value of [29, 3601, 120.5, Number.NaN]) {
			expect(isValidDeployTimeout(value), String(value)).toBe(false)
		}
		expect(() => parseDeployTimeout(1)).toThrow(InvalidDeployTimeout)
	})
})
