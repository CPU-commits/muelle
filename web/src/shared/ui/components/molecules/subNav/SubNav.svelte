<script lang="ts">
	import { link, linkActive, nav } from './styles.css'
	import type { SubNavItem } from './types'

	// Los items pueden ser rutas (`/ajustes/tokens`) o anclas de la misma página (`#seguridad`).
	let { label, items }: { label: string; items: readonly SubNavItem[] } = $props()

	// El link se sigue en esta pestaña: no lo frenó nadie ni lo manda a otra pestaña o ventana
	// una tecla (el botón del medio no dispara `click`).
	function followsHere(event: MouseEvent): boolean {
		return (
			!event.defaultPrevented &&
			event.button === 0 &&
			!(event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
		)
	}
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
<nav aria-label={label} class={nav}>
	{#each items as item (item.href)}
		<a
			href={item.href}
			class={[link, item.active && linkActive]}
			aria-current={item.active ? 'page' : undefined}
			onclick={(event) => {
				if (followsHere(event)) item.onselect?.()
			}}
		>
			{item.label}
		</a>
	{/each}
</nav>
