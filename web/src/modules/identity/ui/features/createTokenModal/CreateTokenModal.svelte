<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Checkbox, Field, Input, LinkButton, Modal, Select } from '@/shared/ui'
	import { ALL_PROJECTS, type CreatedApiToken } from '../../../domain'
	import { CreateTokenModalViewModel } from './CreateTokenModal.viewModel.svelte.ts'
	import { access, accessHint, accessLegend, fields, form as formClass, wide } from './styles.css'

	// Se monta al abrirse (`{#if}` en la pantalla): cada vez empieza vacío.
	type Props = {
		onCreated: (created: CreatedApiToken) => void
		onClose: () => void
	}

	let { onCreated, onClose }: Props = $props()

	const vm = new CreateTokenModalViewModel({ onCreated: (created) => onCreated(created) })
	const formId = $props.id()
	const readOnlyHintId = `${formId}-read-only-hint`
</script>

<Modal
	open
	title={m.identity_token_form_title()}
	description={m.identity_token_form_description()}
	size="lg"
	onClose={() => !vm.isSubmitting && onClose()}
>
	<form
		id={formId}
		class={formClass}
		novalidate
		onsubmit={(event) => {
			event.preventDefault()
			void vm.submit()
		}}
	>
		{#if vm.formError}
			<Banner tone="error" title={vm.formError} />
		{/if}
		{#if vm.showNoProjects}
			<Banner tone="warn" title={m.identity_token_form_no_projects()}>
				{#snippet action()}
					<LinkButton href={resolve('/')} variant="secondary" size="sm">
						{m.identity_token_form_go_projects()}
					</LinkButton>
				{/snippet}
			</Banner>
		{:else if vm.projectsError}
			<Banner tone="error" title={vm.projectsError}>
				{#snippet action()}
					<Button variant="secondary" size="sm" onclick={vm.retryProjects}>
						{m.common_retry()}
					</Button>
				{/snippet}
			</Banner>
		{:else if vm.stagesError}
			<Banner tone="error" title={vm.stagesError}>
				{#snippet action()}
					<Button variant="secondary" size="sm" onclick={vm.retryStages}>
						{m.common_retry()}
					</Button>
				{/snippet}
			</Banner>
		{/if}

		<div class={fields}>
			<div class={wide}>
				<Field label={m.identity_token_form_name()} error={vm.errors.name ?? null}>
					{#snippet children(a11y)}
						<Input
							{...a11y}
							name="name"
							mono
							placeholder="github-actions"
							autocomplete="off"
							spellcheck={false}
							value={vm.name}
							oninput={(event) => vm.setName(event.currentTarget.value)}
						/>
					{/snippet}
				</Field>
			</div>

			<Field
				label={m.identity_token_form_project()}
				hint={vm.isAllProjects ? m.identity_token_form_all_projects_hint() : null}
				error={vm.errors.project ?? null}
			>
				{#snippet children(a11y)}
					<Select
						{...a11y}
						name="project"
						value={vm.project}
						disabled={vm.projectSelectDisabled}
						onchange={(event) => vm.setProject(event.currentTarget.value)}
					>
						<option value="" disabled>
							{vm.isLoadingProjects
								? m.common_loading()
								: m.identity_token_form_project_placeholder()}
						</option>
						<option value={ALL_PROJECTS}>{m.identity_token_form_project_all()}</option>
						{#each vm.projects as project (project.slug)}
							<option value={project.slug}>{project.slug}</option>
						{/each}
					</Select>
				{/snippet}
			</Field>

			<Field label={m.identity_token_form_stage()} error={vm.errors.stage ?? null}>
				{#snippet children(a11y)}
					<Select
						{...a11y}
						name="stage"
						value={vm.stage}
						disabled={vm.stageDisabled}
						onchange={(event) => vm.setStage(event.currentTarget.value)}
					>
						<option value="">
							{vm.isAllProjects
								? m.identity_token_form_stage_none()
								: vm.isLoadingStages
									? m.common_loading()
									: m.identity_token_form_stage_all()}
						</option>
						{#each vm.stages as stage (stage.slug)}
							<option value={stage.slug}>{stage.slug}</option>
						{/each}
					</Select>
				{/snippet}
			</Field>

			<Field
				label={m.identity_token_form_expires()}
				optional
				hint={m.identity_token_form_expires_hint()}
				error={vm.errors.expiresOn ?? null}
			>
				{#snippet children(a11y)}
					<Input
						{...a11y}
						name="expiresOn"
						type="date"
						min={vm.expiryRange.min}
						max={vm.expiryRange.max}
						value={vm.expiresOn}
						oninput={(event) => vm.setExpiresOn(event.currentTarget.value)}
					/>
				{/snippet}
			</Field>

			<fieldset class={[wide, access]}>
				<legend class={accessLegend}>{m.identity_token_form_access()}</legend>
				<Checkbox
					name="readOnly"
					label={m.identity_token_form_read_only()}
					checked={vm.readOnly}
					disabled={vm.readOnlyLocked}
					aria-describedby={readOnlyHintId}
					onchange={(event) => vm.setReadOnly(event.currentTarget.checked)}
				/>
				<p id={readOnlyHintId} class={accessHint}>
					{m.identity_token_form_read_only_hint()}
				</p>
			</fieldset>
		</div>
	</form>

	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={vm.isSubmitting} onclick={onClose}>
			{m.common_cancel()}
		</Button>
		<Button
			type="submit"
			form={formId}
			size="sm"
			loading={vm.isSubmitting}
			disabled={!vm.canSubmit}
		>
			{m.identity_token_form_submit()}
		</Button>
	{/snippet}
</Modal>
