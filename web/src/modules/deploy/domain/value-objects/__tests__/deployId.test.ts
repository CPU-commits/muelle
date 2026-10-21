import { describe, expect, it } from 'vitest'
import { parseDeployId } from '../deployId'

describe('parseDeployId', () => {
	it('lee el ?deploy=144 de la URL', () => {
		expect(parseDeployId('144')).toBe(144)
	})

	it.each([null, undefined, '', '0', '-3', '1.5', 'abc', '14a', '99999999999999999999'])(
		'cualquier otra cosa es ninguno: %j',
		(raw) => {
			expect(parseDeployId(raw)).toBeNull()
		},
	)
})
