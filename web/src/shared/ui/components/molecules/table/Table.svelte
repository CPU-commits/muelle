<script lang="ts">
	import type { Snippet } from 'svelte'
	import { srOnly } from '@/shared/ui/themes/utilities.css'
	import { framed as framedClass, table, wrapper } from './styles.css'

	type Props = {
		/** Con borde y fondo propios. `false` dentro de una <Card flush>. */
		framed?: boolean
		/** Ancho mínimo antes de hacer scroll horizontal (ej. "640px"). */
		minWidth?: string
		/** Nombre de la tabla para lectores de pantalla (no se ve). */
		caption?: string
		children: Snippet
	}

	let { framed = true, minWidth, caption, children }: Props = $props()
</script>

<div class={[wrapper, framed && framedClass]}>
	<table class={table} style:min-width={minWidth}>
		{#if caption}
			<caption class={srOnly}>{caption}</caption>
		{/if}
		{@render children()}
	</table>
</div>
