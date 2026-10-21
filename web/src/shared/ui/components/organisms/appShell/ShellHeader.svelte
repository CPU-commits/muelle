<script lang="ts">
	import type { Snippet } from 'svelte'
	import { getShellSlots } from './shellSlots.svelte.ts'

	// No dibuja nada donde se usa: le pasa sus snippets al header del AppShell mientras
	// está montado. Va en un layout o una página dentro de `(app)`:
	//
	//   <ShellHeader>
	//     {#snippet breadcrumb()}<Breadcrumb … />{/snippet}
	//     {#snippet nav()}<Tabs … />{/snippet}
	//   </ShellHeader>
	type Props = { breadcrumb?: Snippet; nav?: Snippet }

	let { breadcrumb, nav }: Props = $props()

	const slots = getShellSlots()

	// `.pre`: el header cambia antes de pintar, sin un cuadro con la nav anterior. Escribir
	// estado en un efecto es a propósito: los datos van del hijo al AppShell, no hay
	// `$derived` posible. `push` y `release` usan `untrack`, así que el efecto solo depende
	// de las props.
	$effect.pre(() => {
		const token = slots.push({ breadcrumb, nav })
		return () => slots.release(token)
	})
</script>
