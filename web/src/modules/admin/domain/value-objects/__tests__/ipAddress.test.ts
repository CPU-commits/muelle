import { describe, expect, it } from 'vitest'
import { InvalidIpAddress, isIpv4, isIpv6, parseIp, parsePublicIpv4, parsePublicIpv6 } from '../..'

describe('ipAddress', () => {
	it('reconoce IPv4', () => {
		expect(isIpv4('203.0.113.24')).toBe(true)
		expect(isIpv4('0.0.0.0')).toBe(true)
		expect(isIpv4('256.1.1.1')).toBe(false)
		expect(isIpv4('1.2.3')).toBe(false)
		expect(isIpv4('01.2.3.4')).toBe(false)
		expect(parseIp('10.0.0.1')).toEqual({ version: 4, value: 0x0a000001n })
	})

	it('reconoce IPv6, con :: y con una IPv4 al final', () => {
		expect(isIpv6('2001:db8::24')).toBe(true)
		expect(isIpv6('::1')).toBe(true)
		expect(isIpv6('::')).toBe(true)
		expect(isIpv6('2001:0db8:0000:0000:0000:0000:0000:0024')).toBe(true)
		expect(isIpv6('::ffff:192.0.2.1')).toBe(true)
		expect(parseIp('::ffff:192.0.2.1')).toEqual(parseIp('::ffff:c000:201'))
		expect(parseIp('2001:DB8::24')).toEqual(parseIp('2001:db8:0:0:0:0:0:24'))
	})

	it('rechaza IPv6 mal formadas', () => {
		expect(isIpv6('2001:db8::24::1')).toBe(false)
		expect(isIpv6('1:2:3:4:5:6:7')).toBe(false)
		expect(isIpv6('1:2:3:4:5:6:7:8:9')).toBe(false)
		expect(isIpv6('1:2:3:4::5:6:7:8')).toBe(false)
		expect(isIpv6('12345::')).toBe(false)
		expect(isIpv6('fe80::1%eth0')).toBe(false)
		expect(isIpv6('::ffff:999.0.2.1')).toBe(false)
	})

	it('la IPv4 pública es obligatoria y la IPv6 opcional', () => {
		expect(parsePublicIpv4(' 203.0.113.24 ')).toBe('203.0.113.24')
		expect(() => parsePublicIpv4('')).toThrow(InvalidIpAddress)
		expect(() => parsePublicIpv4('2001:db8::1')).toThrow(InvalidIpAddress)
		expect(parsePublicIpv6('')).toBe('')
		expect(parsePublicIpv6('2001:DB8::24')).toBe('2001:db8::24')
		expect(() => parsePublicIpv6('203.0.113.24')).toThrow(
			expect.objectContaining({ name: 'InvalidIpAddress', field: 'ipv6' }),
		)
	})
})
