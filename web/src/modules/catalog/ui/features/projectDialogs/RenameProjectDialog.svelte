<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Field, firstError, Input, Modal } from '@/shared/ui'
	import type { Project } from '../../../domain'
	import { RenameProjectViewModel } from './RenameProject.viewModel.svelte.ts'
	import { form as formClass } from './styles.css'

	let { project, onClose }: { project: Project; onClose: () => void } = $props()

	const vm = new RenameProjectViewModel(
		() => project,
		() => onClose(),
	)
	const formId = $props.id()
</script>

<Modal
	open
	title={m.catalog_rename_project_title({ project: vm.slug })}
	description={m.catalog_rename_project_description({ project: vm.slug })}
	size="sm"
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
							autocomplete="off"
							value={field.state.value}
							onblur={field.handleBlur}
							oninput={(event) => {
								field.handleChange(event.currentTarget.value)
								vm.nameEdited()
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
			{m.common_save()}
		</Button>
	{/snippet}
</Modal>
