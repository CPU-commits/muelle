import { describe, expect, it } from 'vitest'
import type { EnvKey } from '../../value-objects/envKey'
import {
	compareEnvKeys,
	type EnvVar,
	filterEnvVars,
	findEnvVar,
	knownEnvSize,
	overlappingKeys,
	sameEnv,
	sortEnvVars,
} from '../envVar'

const at = new Date('2026-10-01T12:00:00Z')
const plain = (key: string, value: string): EnvVar => ({
	key: key as EnvKey,
	secret: false,
	value,
	updatedAt: at,
})
const secret = (key: string): EnvVar => ({
	key: key as EnvKey,
	secret: true,
	value: null,
	updatedAt: at,
})

describe('envVar', () => {
	it('ordena como el ORDER BY binario de SQLite: mayúsculas, _ y después minúsculas', () => {
		const keys = sortEnvVars([
			plain('b', ''),
			plain('_X', ''),
			plain('B', ''),
			plain('A_B', ''),
		])
		expect(keys.map((envVar) => envVar.key)).toEqual(['A_B', 'B', '_X', 'b'])
		expect(compareEnvKeys('A', 'A')).toBe(0)
	})

	it('no cambia la lista original al ordenar', () => {
		const vars = [plain('B', ''), plain('A', '')]
		sortEnvVars(vars)
		expect(vars.map((envVar) => envVar.key)).toEqual(['B', 'A'])
	})

	it('busca por clave sin distinguir mayúsculas, y por valor solo si no es secreta', () => {
		const vars = [
			plain('DATABASE_URL', 'postgres://db'),
			plain('PORT', '3000'),
			secret('JWT_SECRET'),
		]
		expect(filterEnvVars(vars, 'data').map((envVar) => envVar.key)).toEqual(['DATABASE_URL'])
		expect(filterEnvVars(vars, 'POSTGRES').map((envVar) => envVar.key)).toEqual([
			'DATABASE_URL',
		])
		expect(filterEnvVars(vars, 'jwt').map((envVar) => envVar.key)).toEqual(['JWT_SECRET'])
		expect(filterEnvVars(vars, '  ')).toBe(vars)
	})

	it('el tamaño conocido cuenta la clave de las secretas y clave + valor del resto', () => {
		expect(knownEnvSize([plain('PORT', '3000'), secret('JWT')])).toBe(8 + 3)
	})

	it('las claves que la app pisa del stage', () => {
		const app = [plain('LOG_LEVEL', 'info'), plain('PORT', '3000')]
		const stage = [plain('LOG_LEVEL', 'debug'), secret('DATABASE_URL')]
		expect([...overlappingKeys(app, stage)]).toEqual(['LOG_LEVEL'])
		expect(findEnvVar(app, 'PORT')).toMatchObject({ value: '3000' })
		expect(findEnvVar(app, 'NOPE')).toBeUndefined()
	})

	it('sameEnv: igual si tienen las mismas claves, marcas, valores y fechas', () => {
		const later = new Date('2026-10-05T12:00:00Z')
		const before = [plain('PORT', '3000'), secret('JWT')]
		expect(sameEnv(before, [secret('JWT'), plain('PORT', '3000')])).toBe(true)
		expect(sameEnv(before, [plain('PORT', '8080'), secret('JWT')])).toBe(false)
		expect(sameEnv(before, [plain('PORT', '3000')])).toBe(false)
		expect(sameEnv(before, [plain('PORT', '3000'), plain('JWT', 'x')])).toBe(false)
		// Una secreta reescrita con otro valor solo se nota en la fecha.
		expect(
			sameEnv(before, [plain('PORT', '3000'), { ...secret('JWT'), updatedAt: later }]),
		).toBe(false)
	})
})
