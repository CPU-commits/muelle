import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import ConfirmDialog from '../ConfirmDialog.svelte'

const base = {
	open: true,
	title: 'Eliminar auditfy',
	body: 'Se borra el proyecto. No se puede deshacer.',
	confirmLabel: 'Eliminar',
	cancelLabel: 'Cancelar',
}

describe('ConfirmDialog', () => {
	it('muestra el error del intento adentro del diálogo, sin cerrarlo', async () => {
		const screen = await render(ConfirmDialog, {
			...base,
			error: 'Primero hay que borrar sus stages.',
			onConfirm: vi.fn(),
			onCancel: vi.fn(),
		})

		await expect.element(screen.getByRole('dialog')).toBeVisible()
		await expect
			.element(screen.getByRole('alert'))
			.toHaveTextContent('Primero hay que borrar sus stages.')
	})

	it('sin error no hay alerta; confirmar y cancelar llaman a sus callbacks', async () => {
		const onConfirm = vi.fn()
		const onCancel = vi.fn()
		const screen = await render(ConfirmDialog, { ...base, onConfirm, onCancel })

		await expect.element(screen.getByRole('alert')).not.toBeInTheDocument()
		await screen.getByRole('button', { name: 'Eliminar' }).click()
		await screen.getByRole('button', { name: 'Cancelar' }).click()

		expect(onConfirm).toHaveBeenCalledOnce()
		expect(onCancel).toHaveBeenCalledOnce()
	})
})
