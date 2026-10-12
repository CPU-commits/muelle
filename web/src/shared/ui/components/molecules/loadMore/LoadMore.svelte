<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import Button from '../../atoms/button/Button.svelte'
	import { done, row } from './styles.css'

	// La paginación por cursor (`CursorPage<T>`, la auditoría con `before`): un "Cargar
	// más" al pie de la lista mientras haya cursor. Para listas con total y páginas
	// numeradas está <Pagination>.
	type Props = {
		hasMore: boolean
		loading?: boolean
		onLoadMore: () => void
		/** Texto del botón (por defecto "Cargar más"). */
		label?: string
		/** Lo que se muestra cuando no hay más; sin esto, nada. */
		doneLabel?: string | null
	}

	let { hasMore, loading = false, onLoadMore, label, doneLabel = null }: Props = $props()
</script>

{#if hasMore}
	<div class={row}>
		<Button variant="secondary" size="sm" {loading} onclick={onLoadMore}>
			{label ?? m.common_load_more()}
		</Button>
	</div>
{:else if doneLabel}
	<div class={row}>
		<p class={done}>{doneLabel}</p>
	</div>
{/if}
