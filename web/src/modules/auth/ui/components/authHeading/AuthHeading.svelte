<script lang="ts">
	import { heading, subtitle as subtitleClass, title as titleClass } from './styles.css'

	// El título centrado de las pantallas de acceso. `id` va al `aria-labelledby` del form.
	let { id, title, subtitle }: { id?: string; title: string; subtitle?: string } = $props()
</script>

<div class={heading}>
	<h1 {id} class={titleClass}>{title}</h1>
	{#if subtitle}
		<p class={subtitleClass}>{subtitle}</p>
	{/if}
</div>
