<script lang="ts">
	import type { Snippet } from 'svelte'
	import {
		heights,
		interactive,
		item,
		muted as mutedClass,
		row,
		type ListRowHeight,
	} from './styles.css'

	// Una fila de <List>. Con `href`, la fila entera es un link (las apps de Proyectos);
	// sin él, una fila con sus propios botones (variables, tokens).
	type Props = {
		href?: string
		/** Fondo más oscuro y sin hover: filas heredadas o pendientes. */
		muted?: boolean
		height?: ListRowHeight
		class?: string
		children: Snippet
	}

	let { href, muted = false, height = 'md', class: className, children }: Props = $props()
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
{#if href}
	<li class={item}>
		<a {href} class={[row, heights[height], muted ? mutedClass : interactive, className]}>
			{@render children()}
		</a>
	</li>
{:else}
	<li class={[row, heights[height], muted ? mutedClass : interactive, className]}>
		{@render children()}
	</li>
{/if}
