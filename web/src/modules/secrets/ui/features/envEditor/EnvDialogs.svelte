<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Checkbox, ConfirmDialog, Field, Modal, Textarea } from '@/shared/ui'
	import type { EnvEditorViewModel } from './EnvEditor.viewModel.svelte.ts'
	import { editForm, editNote } from './styles.css'

	// Los diálogos de una fila: editar (o reemplazar, si es secreta), marcar como secreta y
	// borrar.
	let { vm }: { vm: EnvEditorViewModel } = $props()

	const id = $props.id()
</script>

<Modal
	open={vm.editing !== null}
	title={vm.editTitle}
	description={m.secrets_edit_description()}
	onClose={vm.closeEdit}
>
	{#if vm.editing}
		<form
			id={`${id}-edit`}
			class={editForm}
			novalidate
			onsubmit={(event) => {
				event.preventDefault()
				void vm.submitEdit()
			}}
		>
			<Field
				label={vm.editing.secret ? m.secrets_edit_new_value() : m.secrets_edit_value()}
				hint={vm.editing.secret ? m.secrets_edit_secret_hint() : null}
				error={vm.editError}
			>
				{#snippet children(a11y)}
					<Textarea
						{...a11y}
						mono
						rows={4}
						name="value"
						autocomplete="off"
						spellcheck={false}
						value={vm.editValue}
						oninput={(event) => vm.setEditValue(event.currentTarget.value)}
					/>
				{/snippet}
			</Field>
			<Checkbox
				label={m.secrets_edit_secret()}
				checked={vm.editSecret}
				onchange={(event) => vm.setEditSecret(event.currentTarget.checked)}
			/>
			{#if vm.editMakesSecret}
				<p class={editNote}>{m.secrets_edit_to_secret_note()}</p>
			{:else if vm.editConfirmingReveal}
				<Banner tone="warn" title={m.secrets_edit_to_plain_note()} />
			{:else if vm.editRevealsSecret}
				<p class={editNote}>{m.secrets_edit_to_plain_note()}</p>
			{/if}
		</form>
	{/if}

	{#snippet footer()}
		<Button
			variant="secondary"
			size="sm"
			disabled={vm.pendingAction === 'edit'}
			onclick={vm.closeEdit}
		>
			{m.common_cancel()}
		</Button>
		<!-- Dejar a la vista una secreta tiene un segundo paso: el botón lo dice. -->
		<Button
			type="submit"
			form={`${id}-edit`}
			size="sm"
			variant={vm.editConfirmingReveal ? 'danger' : 'primary'}
			loading={vm.pendingAction === 'edit'}
			disabled={!vm.editHasChanges || (vm.isSaving && vm.pendingAction !== 'edit')}
		>
			{vm.editConfirmingReveal ? m.secrets_edit_reveal_confirm() : m.common_save()}
		</Button>
	{/snippet}
</Modal>

<ConfirmDialog
	open={vm.confirmingSecret !== null}
	title={m.secrets_secure_title({ key: vm.confirmingSecret?.key ?? '' })}
	body={m.secrets_secure_body()}
	confirmLabel={m.secrets_secure_confirm()}
	cancelLabel={m.common_cancel()}
	tone="primary"
	loading={vm.pendingAction === 'secure'}
	error={vm.secureError}
	onConfirm={() => void vm.confirmSecure()}
	onCancel={vm.cancelSecure}
/>

<ConfirmDialog
	open={vm.confirmingDelete !== null}
	title={m.secrets_delete_title({ key: vm.confirmingDelete?.key ?? '' })}
	body={vm.deleteBody}
	confirmLabel={m.secrets_delete_confirm()}
	cancelLabel={m.common_cancel()}
	loading={vm.pendingAction === 'delete'}
	error={vm.deleteError}
	onConfirm={() => void vm.confirmDelete()}
	onCancel={vm.cancelDelete}
/>
