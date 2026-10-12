import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import { formatTarget, parseTarget, sameTarget, targetProblem } from '../domainTarget'

describe('destino servicio:puerto', () => {
	it('lee "api:3000" (con espacios alrededor)', () => {
		expect(parseTarget(' api : 3000 ')).toEqual({ service: 'api', port: 3000 })
		expect(formatTarget({ service: 'web', port: 8080 })).toBe('web:8080')
	})

	it.each([
		['api', 'servicio:puerto'],
		['api:', 'servicio:puerto'],
		['API:3000', 'no es un nombre de servicio'],
		['un-servicio-muy-largo:80', 'no es un nombre de servicio'],
		['api:0', 'de 1 a 65535'],
		['api:70000', 'de 1 a 65535'],
	])('rechaza %j', (raw, fragment) => {
		expect(targetProblem(raw)).toContain(fragment)
		expect(() => parseTarget(raw, 'target')).toThrow(ValidationError)
	})

	it('sameTarget compara servicio y puerto', () => {
		expect(sameTarget({ service: 'api', port: 3000 }, { service: 'api', port: 3000 })).toBe(
			true,
		)
		expect(sameTarget({ service: 'api', port: 3000 }, { service: 'api', port: 3001 })).toBe(
			false,
		)
	})
})
