import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import { formatTarget, parseTarget, sameTarget, targetProblem } from '../domainTarget'

describe('destino servicio:puerto', () => {
	it('lee "api:3000" (con espacios alrededor)', () => {
		expect(parseTarget(' api : 3000 ')).toEqual({ service: 'api', port: 3000 })
		expect(formatTarget({ service: 'web', port: 8080 })).toBe('web:8080')
	})

	it.each([
		['api', 'format'],
		['api:', 'format'],
		['API:3000', 'service'],
		['un-servicio-muy-largo:80', 'service'],
		['api:0', 'port'],
		['api:70000', 'port'],
	])('rechaza %j (%s)', (raw, problem) => {
		expect(targetProblem(raw)).toBe(problem)
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
