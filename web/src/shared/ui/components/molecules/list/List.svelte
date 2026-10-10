<script lang="ts">
	import type { Snippet } from 'svelte'
	import { list } from './styles.css'

	// El contenedor; cada fila es un <ListRow>.
	let {
		label,
		class: className,
		children,
	}: { label?: string; class?: string; children: Snippet } = $props()
</script>

<ul class={[list, className]} aria-label={label}>
	{@render children()}
</ul>
