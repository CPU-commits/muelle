import { describe, expect, it, vi } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import CodeInput from '../CodeInput.svelte'

describe('CodeInput', () => {
	it('pegar "123 456" (como lo muestran las apps) deja los 6 dígitos y completa', async () => {
		const oninput = vi.fn()
		const oncomplete = vi.fn()
		const screen = await render(CodeInput, { value: '', label: 'Código', oninput, oncomplete })
		const input = screen.getByRole('textbox', { name: 'Código' })

		await input.click()
		document.execCommand('insertText', false, '123 456')

		expect(oninput).toHaveBeenLastCalledWith('123456')
		expect(oncomplete).toHaveBeenCalledWith('123456')
	})

	it('ignora lo que no es dígito y corta en el largo', async () => {
		const oninput = vi.fn()
		const screen = await render(CodeInput, { value: '', label: 'Código', oninput })

		await userEvent.fill(screen.getByRole('textbox', { name: 'Código' }), '12a34-56789')

		expect(oninput).toHaveBeenLastCalledWith('123456')
	})

	it('muestra cada dígito en su celda', async () => {
		const screen = await render(CodeInput, { value: '481', label: 'Código', oninput: vi.fn() })
		const cells = [...screen.container.querySelectorAll('[aria-hidden="true"] > span')]

		expect(cells.map((cell) => cell.textContent)).toEqual(['4', '8', '1', '', '', ''])
	})
})
