<script lang="ts">
	import Button from '../../atoms/button/Button.svelte'
	import { label as labelClass, pagination } from './styles.css'

	type Props = {
		page: number
		totalPages: number
		label: string
		prevLabel: string
		nextLabel: string
		onPageChange: (page: number) => void
	}

	let { page, totalPages, label, prevLabel, nextLabel, onPageChange }: Props = $props()
</script>

{#if totalPages > 1}
	<nav class={pagination}>
		<Button
			variant="secondary"
			size="sm"
			disabled={page <= 1}
			onclick={() => onPageChange(page - 1)}
		>
			{prevLabel}
		</Button>
		<span class={labelClass}>{label}</span>
		<Button
			variant="secondary"
			size="sm"
			disabled={page >= totalPages}
			onclick={() => onPageChange(page + 1)}
		>
			{nextLabel}
		</Button>
	</nav>
{/if}
