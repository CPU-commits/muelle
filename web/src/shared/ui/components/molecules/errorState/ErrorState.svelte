<script lang="ts">
	import Button from '../../atoms/button/Button.svelte'
	import StatusDot from '../../atoms/statusDot/StatusDot.svelte'
	import { action, body as bodyClass, panel, title as titleClass } from './styles.css'

	// Una carga que falló, con reintento.
	type Props = { title: string; body?: string; retryLabel?: string; onRetry?: () => void }

	let { title, body, retryLabel, onRetry }: Props = $props()
</script>

<div class={panel} role="alert">
	<p class={titleClass}><StatusDot status="bad" />{title}</p>
	{#if body}
		<p class={bodyClass}>{body}</p>
	{/if}
	{#if onRetry && retryLabel}
		<div class={action}>
			<Button variant="secondary" size="sm" onclick={onRetry}>{retryLabel}</Button>
		</div>
	{/if}
</div>
