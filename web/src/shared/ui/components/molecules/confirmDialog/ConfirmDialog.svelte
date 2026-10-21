<script lang="ts">
	import type { Snippet } from 'svelte'
	import Button from '../../atoms/button/Button.svelte'
	import Banner from '../banner/Banner.svelte'
	import Modal from '../modal/Modal.svelte'

	type Props = {
		open: boolean
		title: string
		body: string
		confirmLabel: string
		cancelLabel: string
		loading?: boolean
		/** `danger` (por defecto) para borrar o revocar. */
		tone?: 'primary' | 'danger'
		/**
		 * Por qué falló la confirmación (`not_empty`: "Primero hay que borrar sus stages"). Se
		 * muestra adentro, sin cerrar el diálogo.
		 */
		error?: string | null
		/** Contenido extra entre el texto y los botones (qué se va a borrar). */
		children?: Snippet
		onConfirm: () => void
		onCancel: () => void
	}

	let {
		open,
		title,
		body,
		confirmLabel,
		cancelLabel,
		loading = false,
		tone = 'danger',
		error = null,
		children,
		onConfirm,
		onCancel,
	}: Props = $props()
</script>

<Modal {open} {title} description={body} size="sm" onClose={() => !loading && onCancel()}>
	{@render children?.()}
	{#if error}
		<Banner tone="error" title={error} />
	{/if}
	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={loading} onclick={onCancel}>
			{cancelLabel}
		</Button>
		<Button variant={tone} size="sm" {loading} onclick={onConfirm}>{confirmLabel}</Button>
	{/snippet}
</Modal>
