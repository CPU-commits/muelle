import { yaml } from '@codemirror/lang-yaml'
import { basicSetup, EditorView } from 'codemirror'
import { beforeAll, describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-svelte'
import ShadowHost, { type ShadowMount } from '../ShadowHost.svelte'

// CodeMirror 6 (el editor de compose de catalog) bajo la CSP del panel. El <meta> se suma
// una vez para todo el archivo, después de que vitest ya cargó sus estilos: desde ahí, un
// <style> nuevo en el documento se bloquea, como en producción.
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

function styleViolations(): string[] {
	return violations.filter((violation) => violation.startsWith('style-src'))
}

describe('ShadowHost', () => {
	it('sin shadow root, la CSP frena los estilos que CodeMirror inyecta', async () => {
		const parent = document.createElement('div')
		document.body.append(parent)
		const view = new EditorView({ doc: DOC, extensions: [basicSetup, yaml()], parent })
		await new Promise((done) => setTimeout(done, 50))

		expect(styleViolations().length).toBeGreaterThan(0)
		view.destroy()
		parent.remove()
		violations.length = 0
	})

	it('adentro de un ShadowHost, CodeMirror queda con estilos y sin violar la CSP', async () => {
		let view: EditorView | undefined
		const mount: ShadowMount = (container, root) => {
			view = new EditorView({
				doc: DOC,
				extensions: [basicSetup, yaml()],
				parent: container,
				root,
			})
			return () => view?.destroy()
		}
		const screen = await render(ShadowHost, { mount, 'data-testid': 'compose' })
		await new Promise((done) => setTimeout(done, 50))

		const host = screen.container.querySelector('[data-testid="compose"]')
		const root = host?.shadowRoot
		expect(root?.adoptedStyleSheets.length).toBeGreaterThan(0)
		const gutters = root?.querySelector('.cm-gutters')
		expect(gutters && getComputedStyle(gutters).display).toBe('flex')
		expect(view?.state.doc.toString()).toBe(DOC)
		expect(styleViolations()).toEqual([])
	})

	it('al desmontarse limpia lo que montó', async () => {
		let destroyed = false
		const mount: ShadowMount = (container) => {
			container.textContent = 'adentro'
			return () => {
				destroyed = true
			}
		}
		const screen = await render(ShadowHost, { mount, 'data-testid': 'host' })
		const host = screen.container.querySelector('[data-testid="host"]')
		expect(host?.shadowRoot?.textContent).toBe('adentro')

		screen.unmount()

		expect(destroyed).toBe(true)
	})
})
