<script lang="ts">
	import type { NavItem } from '../tabs/types'
	import { link, linkActive, nav } from './styles.css'

	// Los items pueden ser rutas (`/ajustes/tokens`) o anclas de la misma página (`#seguridad`).
	let { label, items }: { label: string; items: readonly NavItem[] } = $props()
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
<nav aria-label={label} class={nav}>
	{#each items as item (item.href)}
		<a
			href={item.href}
			class={[link, item.active && linkActive]}
			aria-current={item.active ? 'page' : undefined}
		>
			{item.label}
		</a>
	{/each}
</nav>
