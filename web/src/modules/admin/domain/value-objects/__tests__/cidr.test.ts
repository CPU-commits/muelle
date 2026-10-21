import { describe, expect, it } from 'vitest'
import {
	cidrContains,
	cidrListAllows,
	cidrListText,
	InvalidCidr,
	parseCidrList,
	sameCidrList,
} from '../..'

describe('cidr', () => {
	it('lee un rango o una IP por línea, sin líneas vacías', () => {
		expect(parseCidrList('198.51.100.0/24\n\n  203.0.113.7 \n2001:DB8::/32\n')).toEqual([
			'198.51.100.0/24',
			'203.0.113.7',
			'2001:db8::/32',
		])
		expect(parseCidrList('  \n')).toEqual([])
	})

	it('la primera línea inválida sale con su número', () => {
		for (const bad of ['10.0.0/8', '10.0.0.0/33', '2001:db8::/129', '10.0.0.0/8/1', 'x']) {
			let error: unknown
			try {
				parseCidrList(`198.51.100.0/24\n\n${bad}`)
			} catch (caught) {
				error = caught
			}
			expect(error).toBeInstanceOf(InvalidCidr)
			expect((error as InvalidCidr).line).toBe(3)
			expect((error as InvalidCidr).field).toBe('panelAllowedCidrs')
		}
	})

	it('sabe si un rango incluye una IP', () => {
		expect(cidrContains('198.51.100.0/24', '198.51.100.17')).toBe(true)
		expect(cidrContains('198.51.100.0/24', '198.51.101.17')).toBe(false)
		expect(cidrContains('0.0.0.0/0', '8.8.8.8')).toBe(true)
		expect(cidrContains('203.0.113.7', '203.0.113.7')).toBe(true)
		expect(cidrContains('2001:db8::/32', '2001:db8:1::5')).toBe(true)
		expect(cidrContains('2001:db8::/32', '2001:db9::5')).toBe(false)
		expect(cidrContains('2001:db8::/32', '198.51.100.17')).toBe(false)
	})

	it('una lista vacía deja entrar a cualquiera', () => {
		expect(cidrListAllows('', '198.51.100.17')).toBe(true)
		expect(cidrListAllows('10.0.0.0/8\n198.51.100.0/24', '198.51.100.17')).toBe(true)
		expect(cidrListAllows('10.0.0.0/8', '198.51.100.17')).toBe(false)
	})

	it('compara el textarea con la lista guardada sin contar espacios ni líneas vacías', () => {
		const saved = ['198.51.100.0/24', '203.0.113.0/24']
		expect(cidrListText(saved)).toBe('198.51.100.0/24\n203.0.113.0/24')
		expect(sameCidrList(' 198.51.100.0/24\n\n203.0.113.0/24\n', saved)).toBe(true)
		expect(sameCidrList('198.51.100.0/24', saved)).toBe(false)
		expect(sameCidrList('', [])).toBe(true)
	})
})
