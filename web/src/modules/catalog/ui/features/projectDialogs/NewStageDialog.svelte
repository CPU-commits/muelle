<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Field, firstError, Input, Modal } from '@/shared/ui'
	import { type ProjectSlug, STAGE_SLUG_MAX } from '../../../domain'
	import { NewStageViewModel } from './NewStage.viewModel.svelte.ts'
	import { form as formClass } from './styles.css'

	let { project, onClose }: { project: ProjectSlug; onClose: () => void } = $props()

	const vm = new NewStageViewModel(
		() => project,
		() => onClose(),
	)
	const formId = $props.id()
</script>

<Modal
	open
	title={m.catalog_new_stage_title({ project: vm.project })}
	description={m.catalog_new_stage_description()}
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
		<vm.form.Field name="slug">
			{#snippet children(field)}
				<Field
					label={m.catalog_slug()}
					hint={m.catalog_slug_hint({ max: STAGE_SLUG_MAX })}
					error={firstError(field.state.meta.errors) ?? vm.errors.fields.slug ?? null}
				>
					{#snippet children(a11y)}
						<Input
							{...a11y}
							mono
							name={field.name}
							placeholder={m.catalog_stage_slug_placeholder()}
							maxlength={STAGE_SLUG_MAX}
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
