<script lang="ts">
	import Button from '../../atoms/button/Button.svelte'
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
		onConfirm,
		onCancel,
	}: Props = $props()
</script>

<Modal {open} {title} description={body} size="sm" onClose={() => !loading && onCancel()}>
	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={loading} onclick={onCancel}>
			{cancelLabel}
		</Button>
		<Button variant={tone} size="sm" {loading} onclick={onConfirm}>{confirmLabel}</Button>
	{/snippet}
</Modal>
