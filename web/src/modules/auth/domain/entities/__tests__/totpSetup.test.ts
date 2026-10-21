import { describe, expect, it } from 'vitest'
import { groupSecret } from '../..'

describe('groupSecret', () => {
	it('agrupa el secreto de a 4 para copiarlo a mano', () => {
		expect(groupSecret('JBSWY3DPEHPK3PXP')).toBe('JBSW Y3DP EHPK 3PXP')
		expect(groupSecret('ABCDEF')).toBe('ABCD EF')
		expect(groupSecret('AB CD', 2)).toBe('AB CD')
	})
})
