<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { SectionSpy } from './sectionSpy.svelte.ts'
	import SubNav from './SubNav.svelte'
	import { aside, content, layout } from './styles.css'
	import type { SubNavItem } from './types'

	// SubNav a la izquierda y el contenido a la derecha, como Ajustes y Configuración. Con
	// `spy`, mira las secciones del contenido: el `active` de los items lo sigue poniendo quien
	// los arma, con `spy.current`.
	type Props = {
		label: string
		items: readonly SubNavItem[]
		spy?: SectionSpy | undefined
		children: Snippet
	}

	let { label, items, spy, children }: Props = $props()
</script>

<div class={layout}>
	<div class={aside}><SubNav {label} {items} /></div>
	<div class={content} {@attach spy?.track}>{@render children()}</div>
</div>
