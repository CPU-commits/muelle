import { createRawSnippet } from 'svelte'
import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import Button from '../Button.svelte'

const label = createRawSnippet(() => ({ render: () => '<span>Guardar</span>' }))

describe('Button', () => {
	it('cargando no se deshabilita (conserva el foco) pero no responde al clic', async () => {
		const onclick = vi.fn()
		const screen = await render(Button, { children: label, onclick })
		const button = screen.getByRole('button', { name: 'Guardar' })

		;(button.element() as HTMLButtonElement).focus()
		await screen.rerender({ children: label, onclick, loading: true })

		await expect.element(button).toHaveFocus()
		// Sin el atributo `disabled` nativo (aria-disabled sí, para el lector de pantalla).
		expect(button.element().hasAttribute('disabled')).toBe(false)
		await expect.element(button).toHaveAttribute('aria-disabled', 'true')
		;(button.element() as HTMLButtonElement).click()
		expect(onclick).not.toHaveBeenCalled()

		await screen.rerender({ children: label, onclick, loading: false })
		;(button.element() as HTMLButtonElement).click()
		expect(onclick).toHaveBeenCalledOnce()
	})

	it('un submit cargando no vuelve a mandar el formulario', async () => {
		const onsubmit = vi.fn((event: SubmitEvent) => event.preventDefault())
		const form = document.createElement('form')
		form.addEventListener('submit', onsubmit)
		document.body.append(form)
		const screen = await render(Button, {
			target: form,
			props: { children: label, type: 'submit', loading: true },
		})

		;(screen.getByRole('button').element() as HTMLButtonElement).click()

		expect(onsubmit).not.toHaveBeenCalled()
		form.remove()
	})

	it('disabled sigue siendo disabled', async () => {
		const screen = await render(Button, { children: label, disabled: true })

		await expect.element(screen.getByRole('button')).toBeDisabled()
	})
})
