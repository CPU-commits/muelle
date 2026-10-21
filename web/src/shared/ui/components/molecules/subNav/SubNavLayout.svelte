<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { NavItem } from '../tabs/types'
	import SubNav from './SubNav.svelte'
	import { aside, content, layout } from './styles.css'

	// SubNav a la izquierda y el contenido a la derecha, como Ajustes y Configuración.
	type Props = { label: string; items: readonly NavItem[]; children: Snippet }

	let { label, items, children }: Props = $props()
</script>

<div class={layout}>
	<div class={aside}><SubNav {label} {items} /></div>
	<div class={content}>{@render children()}</div>
</div>
