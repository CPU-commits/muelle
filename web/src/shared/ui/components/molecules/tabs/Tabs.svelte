<script lang="ts">
	import { nav, tab, tabActive } from './styles.css'
	import type { NavItem } from './types'

	// `label` nombra la nav para lectores de pantalla ("Secciones", "Secciones de la app").
	let { label, items }: { label: string; items: readonly NavItem[] } = $props()
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
<nav aria-label={label} class={nav}>
	{#each items as item (item.href)}
		<a
			href={item.href}
			class={[tab, item.active && tabActive]}
			aria-current={item.active ? 'page' : undefined}
		>
			{item.label}
		</a>
	{/each}
</nav>
