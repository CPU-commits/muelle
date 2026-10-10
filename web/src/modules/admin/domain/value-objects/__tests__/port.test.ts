import { describe, expect, it } from 'vitest'
import { InvalidPort, parseProxyPorts } from '../..'

function problemOf(http: string, https: string): [string | undefined, string] | null {
	try {
		parseProxyPorts(http, https)
		return null
	} catch (error) {
		return [(error as InvalidPort).field, (error as InvalidPort).problem]
	}
}

describe('port', () => {
	it('acepta dos puertos distintos entre 1 y 65535', () => {
		expect(parseProxyPorts('80', ' 443 ')).toEqual({ http: 80, https: 443 })
		expect(parseProxyPorts('8080', '65535')).toEqual({ http: 8080, https: 65535 })
	})

	it('dice qué campo está mal', () => {
		expect(problemOf('0', '443')).toEqual(['httpPort', 'format'])
		expect(problemOf('80', '65536')).toEqual(['httpsPort', 'format'])
		expect(problemOf('8o', '443')).toEqual(['httpPort', 'format'])
		expect(problemOf('80.5', '443')).toEqual(['httpPort', 'format'])
		expect(problemOf('443', '443')).toEqual(['httpsPort', 'same'])
		expect(() => parseProxyPorts('', '443')).toThrow(InvalidPort)
	})
})
