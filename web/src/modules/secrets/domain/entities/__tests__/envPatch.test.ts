import { describe, expect, it } from 'vitest'
import {
	EmptyEnvPatch,
	EnvKeyInSetAndUnset,
	EnvLimitExceeded,
	InvalidEnvKey,
	InvalidEnvValue,
} from '../../errors/secretsErrors'
import type { EnvKey } from '../../value-objects/envKey'
import { MAX_ENV_VARS, MAX_OWNER_BYTES, MAX_VALUE_BYTES } from '../../value-objects/envLimits'
import { parseEnvPatch, parseEnvReplacement, patchLimitProblem } from '../envPatch'
import type { EnvVar } from '../envVar'

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

function caught(run: () => unknown): unknown {
	try {
		run()
	} catch (error) {
		return error
	}
	throw new Error('no tiró')
}

const many = (count: number, prefix = 'K'): [string, string][] =>
	Array.from({ length: count }, (_, index) => [`${prefix}${index}`, 'v'])

describe('parseEnvPatch (PatchEnv.Execute)', () => {
	it('un PATCH vacío se rechaza en set', () => {
		expect(caught(() => parseEnvPatch({ secret: true }))).toBeInstanceOf(EmptyEnvPatch)
		expect(caught(() => parseEnvPatch({ set: [], unset: [], secret: true }))).toMatchObject({
			field: 'set',
		})
	})

	it('ordena y saca repetidos de unset; set queda en un Map (también __proto__)', () => {
		const patch = parseEnvPatch({
			set: [
				['__proto__', 'x'],
				['B', '2'],
			],
			unset: ['Z', 'A', 'Z'],
			secret: false,
		})
		expect(patch.unset).toEqual(['A', 'Z'])
		expect([...patch.set]).toEqual([
			['B', '2'],
			['__proto__', 'x'],
		])
		expect(patch.secret).toBe(false)
	})

	it('los límites de la petición: 500 en unset y 500 en set', () => {
		expect(
			caught(() =>
				parseEnvPatch({ unset: many(MAX_ENV_VARS + 1).map(([k]) => k), secret: true }),
			),
		).toMatchObject({ limit: 'count', field: 'unset' })
		expect(
			caught(() => parseEnvPatch({ set: many(MAX_ENV_VARS + 1), secret: true })),
		).toMatchObject({ limit: 'count', field: 'set' })
		expect(() => parseEnvPatch({ set: many(MAX_ENV_VARS), secret: true })).not.toThrow()
	})

	it('valida las claves de unset y que no estén también en set', () => {
		expect(
			caught(() => parseEnvPatch({ unset: ['COMPOSE_FILE'], secret: true })),
		).toMatchObject({
			problem: 'reserved',
			field: 'unset',
		})
		const both = caught(() => parseEnvPatch({ set: [['A', '1']], unset: ['A'], secret: true }))
		expect(both).toBeInstanceOf(EnvKeyInSetAndUnset)
		expect(both).toMatchObject({ key: 'A', field: 'unset' })
	})

	it('valida set en orden de clave: el primer error es el que daría el backend', () => {
		const error = caught(() =>
			parseEnvPatch({
				set: [
					['Z-BAD', '1'],
					['A-BAD', '1'],
				],
				secret: true,
			}),
		)
		expect(error).toBeInstanceOf(InvalidEnvKey)
		expect(error).toMatchObject({ key: 'A-BAD', field: 'set' })
	})

	it('valida los valores', () => {
		expect(
			caught(() =>
				parseEnvPatch({ set: [['BIG', 'a'.repeat(MAX_VALUE_BYTES + 1)]], secret: true }),
			),
		).toMatchObject({ problem: 'too_large', key: 'BIG' })
		const nul = caught(() => parseEnvPatch({ set: [['NUL', 'a\0']], secret: true }))
		expect(nul).toBeInstanceOf(InvalidEnvValue)
		expect(nul).toMatchObject({ problem: 'nul', field: 'set' })
	})
})

describe('parseEnvReplacement (ReplaceEnv.Execute)', () => {
	it('acepta un set vacío: deja al dueño sin variables', () => {
		expect(parseEnvReplacement({ vars: [], secret: true }).vars.size).toBe(0)
	})

	it('cantidad, cada variable y el total, en el campo vars', () => {
		expect(caught(() => parseEnvReplacement({ vars: many(501), secret: true }))).toMatchObject({
			limit: 'count',
			field: 'vars',
		})
		expect(
			caught(() => parseEnvReplacement({ vars: [['1X', '']], secret: true })),
		).toMatchObject({
			problem: 'format',
			field: 'vars',
		})
		// 17 valores de 64 KiB superan 1 MiB aunque cada uno esté en el límite.
		const big = Array.from({ length: 17 }, (_, index): [string, string] => [
			`K${index}`,
			'a'.repeat(MAX_VALUE_BYTES),
		])
		const error = caught(() => parseEnvReplacement({ vars: big, secret: false }))
		expect(error).toBeInstanceOf(EnvLimitExceeded)
		expect(error).toMatchObject({ limit: 'size', field: 'vars' })
	})
})

describe('patchLimitProblem (los límites del dueño con lo que sabe el panel)', () => {
	it('cuenta lo que queda: lo de antes, menos unset, más set', () => {
		const current = many(MAX_ENV_VARS).map(([key]) => plain(key, 'v'))
		const add = parseEnvPatch({ set: [['NEW', 'v']], secret: true })
		expect(patchLimitProblem(current, add)).toBe('count')

		const swap = parseEnvPatch({ set: [['NEW', 'v']], unset: ['K0'], secret: true })
		expect(patchLimitProblem(current, swap)).toBeNull()

		const replaceExisting = parseEnvPatch({ set: [['K1', 'otro']], secret: true })
		expect(patchLimitProblem(current, replaceExisting)).toBeNull()
	})

	it('borrar siempre se permite, aunque el dueño ya esté pasado', () => {
		const current = many(MAX_ENV_VARS + 10).map(([key]) => plain(key, 'v'))
		expect(
			patchLimitProblem(current, parseEnvPatch({ unset: ['K0'], secret: true })),
		).toBeNull()
	})

	it('el tamaño cuenta los valores conocidos; de una secreta, solo la clave', () => {
		const half = 'a'.repeat(MAX_VALUE_BYTES)
		const current = Array.from({ length: 14 }, (_, index) => plain(`P${index}`, half))
		const fits = parseEnvPatch({ set: [['NEW', half]], secret: true })
		expect(patchLimitProblem(current, fits)).toBeNull()

		const tooMuch = parseEnvPatch({
			set: [
				['NEW', half],
				['NEW2', half],
			],
			secret: true,
		})
		expect(patchLimitProblem(current, tooMuch)).toBe('size')

		// Las secretas no suman su valor (no se conoce): el backend es quien decide.
		const secrets = Array.from({ length: 14 }, (_, index) => secret(`S${index}`))
		expect(patchLimitProblem(secrets, tooMuch)).toBeNull()
		expect(MAX_OWNER_BYTES).toBe(16 * MAX_VALUE_BYTES)
	})
})
