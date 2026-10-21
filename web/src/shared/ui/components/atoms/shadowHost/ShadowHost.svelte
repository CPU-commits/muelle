<script lang="ts" module>
	/**
	 * Monta la librería adentro del shadow root: recibe el contenedor donde dibujar y el
	 * shadow root (lo que CodeMirror pide como `root`). Devuelve la limpieza.
	 */
	export type ShadowMount = (container: HTMLElement, root: ShadowRoot) => void | (() => void)
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements'
	import type { Attachment } from 'svelte/attachments'

	// Para librerías que inyectan sus estilos con un <style> (CodeMirror 6 y su style-mod):
	// la CSP del panel (`style-src 'self'`) los bloquea en el documento. Adentro de un
	// shadow root esas librerías usan `adoptedStyleSheets` (hojas construidas por JS), que
	// la CSP no frena. Los tokens del tema (`theme.*`, variables de :root) se heredan
	// adentro; las clases de vanilla-extract no: lo de adentro se estila con lo de la
	// librería (`EditorView.theme`) y el borde o el tamaño, con `class` en el host.
	type Props = HTMLAttributes<HTMLDivElement> & {
		/** Tiene que ser estable (una arrow function del viewModel): si cambia, se vuelve a montar. */
		mount: ShadowMount
	}

	let { mount, ...rest }: Props = $props()

	function inShadowRoot(setup: ShadowMount): Attachment<HTMLDivElement> {
		return (host) => {
			const root = host.shadowRoot ?? host.attachShadow({ mode: 'open' })
			const container = document.createElement('div')
			root.append(container)
			const cleanup = setup(container, root)
			return () => {
				cleanup?.()
				container.remove()
			}
		}
	}
</script>

<div {...rest} {@attach inShadowRoot(mount)}></div>
