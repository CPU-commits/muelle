import { afterEach, describe, expect, it, vi } from 'vitest'
import { toast, toasts } from '../toasts.svelte.ts'

describe('toast', () => {
	afterEach(() => {
		vi.unstubAllGlobals()
		for (const item of [...toasts.items]) toasts.dismiss(item.id)
	})

	// `crypto.randomUUID` solo existe en un contexto seguro: el panel por HTTP en una IP de
	// la red local no lo tiene, y un aviso no puede romper por eso.
	it('funciona sin crypto.randomUUID y cada aviso tiene su id', () => {
		vi.stubGlobal('crypto', {})

		toast.success('Guardado')
		toast.error('No se pudo guardar')

		const ids = toasts.items.map((item) => item.id)
		expect(ids).toHaveLength(2)
		expect(new Set(ids).size).toBe(2)
	})
})
