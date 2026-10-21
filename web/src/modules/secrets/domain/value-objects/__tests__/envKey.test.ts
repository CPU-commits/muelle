import { describe, expect, it } from 'vitest'
import { InvalidEnvKey } from '../../errors/secretsErrors'
import { envKeyProblem, isValidEnvKey, normalizeEnvKey, parseEnvKey } from '../envKey'

describe('envKey (validate.EnvKey del backend)', () => {
	it('acepta letras, números y _, sin empezar con número', () => {
		for (const key of [
			'PORT',
			'database_url',
			'_PRIVATE',
			'A1',
			'a',
			'__proto__',
			'compose_x',
		]) {
			expect(envKeyProblem(key), key).toBeNull()
		}
	})

	it('rechaza el formato con el mismo criterio que la regex del backend', () => {
		for (const key of ['1PORT', 'MY-VAR', 'MY.VAR', 'MY VAR', 'Ñ', 'KEY=', ' PORT']) {
			expect(envKeyProblem(key), key).toBe('format')
		}
	})

	it('hasta 128 caracteres', () => {
		expect(envKeyProblem('A'.repeat(128))).toBeNull()
		expect(envKeyProblem('A'.repeat(129))).toBe('too_long')
	})

	it('el prefijo COMPOSE_ está reservado, solo en mayúsculas', () => {
		expect(envKeyProblem('COMPOSE_PROFILES')).toBe('reserved')
		expect(envKeyProblem('COMPOSE_')).toBe('reserved')
		expect(envKeyProblem('COMPOSER_HOME')).toBeNull()
		expect(envKeyProblem('Compose_x')).toBeNull()
	})

	it('vacía', () => {
		expect(envKeyProblem('')).toBe('empty')
	})

	it('normalize recorta lo que escribe una persona; parse no recorta', () => {
		expect(normalizeEnvKey('  PORT \t')).toBe('PORT')
		expect(() => parseEnvKey(' PORT', 'set')).toThrow(InvalidEnvKey)
	})

	it('parse tira InvalidEnvKey con el problema, la clave y el campo del cuerpo', () => {
		const error = (() => {
			try {
				parseEnvKey('COMPOSE_FILE', 'unset')
			} catch (caught) {
				return caught
			}
		})()
		expect(error).toBeInstanceOf(InvalidEnvKey)
		expect(error).toMatchObject({ problem: 'reserved', key: 'COMPOSE_FILE', field: 'unset' })
		expect(parseEnvKey('PORT', 'set')).toBe('PORT')
		expect(isValidEnvKey('PORT')).toBe(true)
		expect(isValidEnvKey('1')).toBe(false)
	})
})
