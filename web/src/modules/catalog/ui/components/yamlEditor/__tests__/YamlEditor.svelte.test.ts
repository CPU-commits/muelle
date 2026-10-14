import { EditorView } from '@codemirror/view'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import YamlEditor from '../YamlEditor.svelte'

// El editor de compose bajo la CSP del panel (`style-src 'self'`): el <meta> se suma una
// vez, después de que vitest cargó sus estilos; desde ahí un <style> nuevo en el
// documento se bloquea, como en producción.
const violations: string[] = []

beforeAll(async () => {
	document.addEventListener('securitypolicyviolation', (event) => {
		violations.push(`${event.effectiveDirective}: ${event.blockedURI}`)
	})
	const meta = document.createElement('meta')
	meta.httpEquiv = 'Content-Security-Policy'
	meta.content = "style-src 'self'"
	document.head.append(meta)
	await new Promise((done) => setTimeout(done, 20))
})

const DOC = 'services:\n  api:\n    image: nginx:1.27\n'
const tick = () => new Promise((done) => setTimeout(done, 30))

async function mountEditor(props: Partial<Parameters<typeof render<typeof YamlEditor>>[1]> = {}) {
	const onChange = vi.fn<(value: string) => void>()
	const screen = await render(YamlEditor, {
		value: DOC,
		label: 'compose.yaml',
		description: 'Esc y después Tab sale del editor',
		onChange,
		...props,
	})
	await tick()
	const root = screen.container.querySelector('div')?.shadowRoot
	const content = root?.querySelector<HTMLElement>('.cm-content')
	if (!root || !content) throw new Error('el editor no se montó en un shadow root')
	const view = EditorView.findFromDOM(content)
	if (!view) throw new Error('no hay EditorView')
	return { screen, root, content, view, onChange }
}

describe('YamlEditor', () => {
	it('se monta en el shadow root con su tema y sin violar la CSP', async () => {
		const { root, content, view } = await mountEditor()

		expect(view.state.doc.toString()).toBe(DOC)
		expect(root.adoptedStyleSheets.length).toBeGreaterThan(0)
		// El de CodeMirror (el scroller es flex) y el nuestro (editorTheme.ts).
		const scroller = root.querySelector('.cm-scroller') as Element
		expect(getComputedStyle(scroller).display).toBe('flex')
		expect(getComputedStyle(scroller).minHeight).toBe('320px')
		expect(content.getAttribute('aria-label')).toBe('compose.yaml')
		expect(content.getAttribute('aria-description')).toBe('Esc y después Tab sale del editor')
		expect(violations.filter((violation) => violation.startsWith('style-src'))).toEqual([])
	})

	it('cada cambio sale por onChange', async () => {
		const { view, onChange } = await mountEditor()

		view.dispatch({ changes: { from: 0, insert: '# hola\n' } })

		expect(onChange).toHaveBeenLastCalledWith(`# hola\n${DOC}`)
	})

	it('un value nuevo desde afuera reemplaza el documento, sin volver a montarlo', async () => {
		const { screen, view, root } = await mountEditor()
		const next = 'services:\n  web:\n    image: nginx:1.28\n'

		await screen.rerender({ value: next })
		await tick()

		expect(view.state.doc.toString()).toBe(next)
		expect(root.querySelectorAll('.cm-editor')).toHaveLength(1)
	})

	it('marca las líneas con error y lleva el cursor a la que se pide', async () => {
		const { screen, view, root } = await mountEditor()

		await screen.rerender({ errorLines: [2], focusRequest: { line: 3, nonce: 1 } })
		await tick()

		const marked = [...root.querySelectorAll('.cm-line')].map((line) =>
			line.classList.contains('cm-muelle-error-line'),
		)
		expect(marked.slice(0, 3)).toEqual([false, true, false])
		expect(view.state.selection.main.head).toBe(view.state.doc.line(3).from)
		expect(view.hasFocus).toBe(true)
	})

	it('al desmontarse destruye el editor', async () => {
		const { screen, view } = await mountEditor()

		screen.unmount()

		expect(view.dom.isConnected).toBe(false)
	})
})
