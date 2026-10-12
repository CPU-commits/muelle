import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { toast, toasts } from '@/shared/ui/stores'
import Toaster from '../Toaster.svelte'

describe('Toaster', () => {
	beforeEach(() => {
		vi.useFakeTimers()
		toasts.items = []
	})

	afterEach(() => {
		vi.useRealTimers()
		toasts.items = []
	})

	it('es una sola región viva, sin roles anidados', async () => {
		const screen = await render(Toaster)
		toast.success('Guardado')
		toast.error('No se pudo guardar')
		await vi.advanceTimersByTimeAsync(0)

		expect(screen.container.querySelectorAll('[aria-live]')).toHaveLength(1)
		expect(screen.container.querySelectorAll('[role="status"], [role="alert"]')).toHaveLength(0)
		await expect.element(screen.getByText('No se pudo guardar')).toBeInTheDocument()
	})

	it('un aviso se va solo; un error se queda hasta que se cierra', async () => {
		const screen = await render(Toaster)
		toast.success('Guardado')
		toast.error('No se pudo guardar')

		await vi.advanceTimersByTimeAsync(10_000)

		await expect.element(screen.getByText('Guardado')).not.toBeInTheDocument()
		await expect.element(screen.getByText('No se pudo guardar')).toBeInTheDocument()
		await screen.getByRole('button', { name: 'Cerrar' }).click()
		await expect.element(screen.getByText('No se pudo guardar')).not.toBeInTheDocument()
	})

	it('con el puntero o el foco encima, el aviso espera', async () => {
		const screen = await render(Toaster)
		toast.success('Guardado')
		await vi.advanceTimersByTimeAsync(1_000)

		const item = screen.getByText('Guardado').element().closest('[data-toast]') as HTMLElement
		item.dispatchEvent(new MouseEvent('mouseenter'))
		await vi.advanceTimersByTimeAsync(10_000)
		await expect.element(screen.getByText('Guardado')).toBeInTheDocument()

		item.dispatchEvent(new MouseEvent('mouseleave'))
		await vi.advanceTimersByTimeAsync(5_000)
		await expect.element(screen.getByText('Guardado')).not.toBeInTheDocument()
	})
})
