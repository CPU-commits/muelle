<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Field, firstError, Input, Modal } from '@/shared/ui'
	import { PROJECT_SLUG_MAX } from '../../../domain'
	import { NewProjectViewModel } from './NewProject.viewModel.svelte.ts'
	import { form as formClass } from './styles.css'

	// Se monta al abrirse (y se desmonta al cerrar): cada vez arranca vacío.
	let { onClose }: { onClose: () => void } = $props()

	const vm = new NewProjectViewModel(() => onClose())
	const formId = $props.id()
</script>

<Modal
	open
	title={m.catalog_new_project_title()}
	description={m.catalog_new_project_description()}
	onClose={() => !vm.isPending && onClose()}
>
	<form
		id={formId}
		class={formClass}
		novalidate
		onsubmit={(event) => {
			event.preventDefault()
			event.stopPropagation()
			void vm.form.handleSubmit()
		}}
	>
		{#if vm.errors.form}
			<Banner tone="error" title={vm.errors.form} />
		{/if}

		<vm.form.Field name="name">
			{#snippet children(field)}
				<Field
					label={m.catalog_project_name()}
					error={firstError(field.state.meta.errors) ?? vm.errors.fields.name ?? null}
				>
					{#snippet children(a11y)}
						<Input
							{...a11y}
							name={field.name}
							placeholder={m.catalog_project_name_placeholder()}
							autocomplete="off"
							value={field.state.value}
							onblur={field.handleBlur}
							oninput={(event) => {
								field.handleChange(event.currentTarget.value)
								vm.nameEdited(event.currentTarget.value)
							}}
						/>
					{/snippet}
				</Field>
			{/snippet}
		</vm.form.Field>

		<vm.form.Field name="slug">
			{#snippet children(field)}
				<Field
					label={m.catalog_slug()}
					hint={m.catalog_slug_hint({ max: PROJECT_SLUG_MAX })}
					error={firstError(field.state.meta.errors) ?? vm.errors.fields.slug ?? null}
				>
					{#snippet children(a11y)}
						<Input
							{...a11y}
							mono
							name={field.name}
							placeholder="auditfy"
							maxlength={PROJECT_SLUG_MAX}
							autocomplete="off"
							autocapitalize="none"
							spellcheck={false}
							value={field.state.value}
							onblur={field.handleBlur}
							oninput={(event) => {
								field.handleChange(event.currentTarget.value)
								vm.slugEdited()
							}}
						/>
					{/snippet}
				</Field>
			{/snippet}
		</vm.form.Field>
	</form>

	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={vm.isPending} onclick={onClose}>
			{m.common_cancel()}
		</Button>
		<Button type="submit" form={formId} size="sm" loading={vm.isPending}>
			{m.common_create()}
		</Button>
	{/snippet}
</Modal>
