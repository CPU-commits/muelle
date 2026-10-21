import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import { hostnameProblem, normalizeHostname, parseHostname } from '../hostname'

describe('hostname', () => {
	it.each([
		'api-staging.tudominio.cl',
		'api.staging.otro-dominio.com',
		'a.io',
		'xn--bcher-kva.example',
	])('acepta %s', (raw) => {
		expect(hostnameProblem(raw)).toBeNull()
	})

	it('normaliza mayúsculas, espacios y el punto final', () => {
		expect(normalizeHostname('  API.TuDominio.cl. ')).toBe('api.tudominio.cl')
		expect(parseHostname('API.TuDominio.cl.')).toBe('api.tudominio.cl')
	})

	it.each([
		['', 'empty'],
		['localhost', 'no_domain'],
		['https://api.tudominio.cl', 'not_bare'],
		['api.tudominio.cl/v1', 'not_bare'],
		['-api.tudominio.cl', 'bad_label'],
		['api_v1.tudominio.cl', 'bad_label'],
		['api.tudominio.123', 'bad_tld'],
		['*.tudominio.cl', 'bad_label'],
	])('rechaza %j (%s)', (raw, problem) => {
		expect(hostnameProblem(raw)).toBe(problem)
		expect(() => parseHostname(raw)).toThrow(ValidationError)
	})

	it('demasiado largo', () => {
		const long = `${'a'.repeat(60)}.`.repeat(5) + 'cl'
		expect(hostnameProblem(long)).toBe('too_long')
	})

	it('el error lleva el campo', () => {
		try {
			parseHostname('nada')
			expect.unreachable()
		} catch (error) {
			expect((error as ValidationError).field).toBe('hostname')
		}
	})
})
