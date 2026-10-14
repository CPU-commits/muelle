import { describe, expect, it } from 'vitest'
import type { EnvVar } from '../../entities/envVar'
import type { EnvKey } from '../../value-objects/envKey'
import { type ImportEntry, planImport } from '../importPlan'

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
const entry = (key: string, value: string): ImportEntry => ({ key: key as EnvKey, value })

const current = [plain('PORT', '3000'), plain('LOG_LEVEL', 'info'), secret('JWT_SECRET')]

describe('planImport', () => {
	it('combinar sin secretas: nuevas, cambian, sin cambios; lo demás queda', () => {
		const plan = planImport(
			current,
			[entry('PORT', '3000'), entry('LOG_LEVEL', 'debug'), entry('NEW', 'x')],
			{ mode: 'merge', secret: false },
		)
		expect(plan.added).toEqual(['NEW'])
		expect(plan.changed).toEqual([{ key: 'LOG_LEVEL', reason: 'value' }])
		expect(plan.unchanged).toEqual(['PORT'])
		expect(plan.overwritten).toEqual([])
		expect(plan.removed).toEqual([])
		expect(plan.patch).toEqual({
			set: new Map([
				['LOG_LEVEL', 'debug'],
				['NEW', 'x'],
			]),
			unset: [],
			secret: false,
		})
	})

	it('una secreta guardada no se puede comparar: se reescribe', () => {
		const plan = planImport(current, [entry('JWT_SECRET', 'otro')], {
			mode: 'merge',
			secret: true,
		})
		expect(plan.overwritten).toEqual(['JWT_SECRET'])
		expect(plan.changed).toEqual([])
		expect(plan.patch?.set).toEqual(new Map([['JWT_SECRET', 'otro']]))
	})

	it('cambiar la marca de secreta es un cambio, aunque el valor sea el mismo', () => {
		const toSecret = planImport(current, [entry('PORT', '3000')], {
			mode: 'merge',
			secret: true,
		})
		expect(toSecret.changed).toEqual([{ key: 'PORT', reason: 'to_secret' }])
		expect(toSecret.unchanged).toEqual([])

		const toPlain = planImport(current, [entry('JWT_SECRET', 'x')], {
			mode: 'merge',
			secret: false,
		})
		expect(toPlain.changed).toEqual([{ key: 'JWT_SECRET', reason: 'to_plain' }])
		expect(toPlain.overwritten).toEqual([])
	})

	it('reemplazar todas borra las que no están en el archivo', () => {
		const plan = planImport(current, [entry('PORT', '3000')], {
			mode: 'replace',
			secret: false,
		})
		expect(plan.removed).toEqual(['JWT_SECRET', 'LOG_LEVEL'])
		expect(plan.unchanged).toEqual(['PORT'])
		expect(plan.patch).toEqual({
			set: new Map(),
			unset: ['JWT_SECRET', 'LOG_LEVEL'],
			secret: false,
		})
	})

	it('sin nada que cambiar no hay PATCH', () => {
		expect(
			planImport(current, [entry('PORT', '3000')], { mode: 'merge', secret: false }).patch,
		).toBeNull()
		expect(planImport([], [], { mode: 'replace', secret: true }).patch).toBeNull()
	})

	it('las listas salen ordenadas por clave', () => {
		const plan = planImport([], [entry('Z', '1'), entry('A', '1'), entry('M', '1')], {
			mode: 'merge',
			secret: true,
		})
		expect(plan.added).toEqual(['A', 'M', 'Z'])
		expect([...(plan.patch?.set.keys() ?? [])]).toEqual(['A', 'M', 'Z'])
	})
})
