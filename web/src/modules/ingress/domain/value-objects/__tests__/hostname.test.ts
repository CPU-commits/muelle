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
		['', 'vacío'],
		['localhost', 'le falta el dominio'],
		['https://api.tudominio.cl', 'sin https://'],
		['api.tudominio.cl/v1', 'sin https://'],
		['-api.tudominio.cl', 'guiones'],
		['api_v1.tudominio.cl', 'guiones'],
		['api.tudominio.123', 'primer nivel'],
		['*.tudominio.cl', 'guiones'],
	])('rechaza %j', (raw, fragment) => {
		expect(hostnameProblem(raw)).toContain(fragment)
		expect(() => parseHostname(raw)).toThrow(ValidationError)
	})

	it('demasiado largo', () => {
		const long = `${'a'.repeat(60)}.`.repeat(5) + 'cl'
		expect(hostnameProblem(long)).toBe('es demasiado largo')
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
