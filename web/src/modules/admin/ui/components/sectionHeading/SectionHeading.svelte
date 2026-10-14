<script lang="ts">
	import type { Snippet } from 'svelte'
	import {
		actions as actionsClass,
		description as descriptionClass,
		heading,
		text,
		title as titleClass,
	} from './styles.css'

	// `id` va en el h2: la sección lo usa en su `aria-labelledby`.
	type Props = { id: string; title: string; description?: string; actions?: Snippet }

	let { id, title, description, actions }: Props = $props()
</script>

<div class={heading}>
	<div class={text}>
		<h2 {id} class={titleClass}>{title}</h2>
		{#if description}
			<p class={descriptionClass}>{description}</p>
		{/if}
	</div>
	{#if actions}
		<div class={actionsClass}>{@render actions()}</div>
	{/if}
</div>
