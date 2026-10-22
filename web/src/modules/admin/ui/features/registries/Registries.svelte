<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Banner,
		Button,
		ConfirmDialog,
		EmptyState,
		ErrorState,
		Field,
		Input,
		List,
		ListRow,
		LoadingState,
		Menu,
		Modal,
		Mono,
		srOnly,
	} from '@/shared/ui'
	import { SectionHeading } from '../../components/sectionHeading'
	import { RegistriesViewModel } from './Registries.viewModel.svelte.ts'
	import { dateCell, form as formClass, hostCell, section, userCell } from './styles.css'

	// La vista "Registries" de Ajustes (`Ajustes.dc.html`).
	const vm = new RegistriesViewModel()
	const titleId = $props.id()
	const formId = `${titleId}-form`
</script>

<section class={section} aria-labelledby={titleId}>
	<SectionHeading
		id={titleId}
		title={m.admin_registries_title()}
		description={m.admin_registries_description()}
	>
		{#snippet actions()}
			{#if !vm.isLoading && !vm.loadError}
				<Button variant="secondary" size="sm" onclick={vm.openAdd}>
					{m.admin_registries_add()}
				</Button>
			{/if}
		{/snippet}
	</SectionHeading>

	{#if vm.isLoading}
		<LoadingState label={m.common_loading()} />
	{:else if vm.loadError}
		<ErrorState title={vm.loadError} retryLabel={m.common_retry()} onRetry={vm.retry} />
	{:else if vm.isEmpty}
		<EmptyState
			title={m.admin_registries_empty_title()}
			body={m.admin_registries_empty_body()}
		/>
	{:else}
		<List label={m.admin_registries_title()}>
			{#each vm.registries as registry (registry.host)}
				<ListRow>
					<span class={hostCell}><Mono>{registry.host}</Mono></span>
					<span class={userCell}>{registry.username}</span>
					<!-- "Actualizado hace 3 días" a la vista; la fecha exacta en el `title` para el
					     puntero y en el texto para un lector de pantalla. -->
					<span class={dateCell}>
						<time
							datetime={registry.updatedAt.toISOString()}
							title={vm.updatedTitle(registry)}
						>
							{vm.updatedLabel(registry)}
						</time>
						<span class={srOnly}>{vm.updatedExact(registry)}</span>
					</span>
					<Menu
						label={m.admin_registries_options({ host: registry.host })}
						items={vm.menuFor(registry)}
					/>
				</ListRow>
			{/each}
		</List>
	{/if}
</section>

<Modal
	open={vm.dialog !== null}
	title={vm.dialogTitle}
	description={m.admin_registry_form_description()}
	onClose={vm.closeDialog}
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
		{#if vm.dialog?.mode === 'add'}
			<Field
				label={m.admin_registry_form_host()}
				hint={m.admin_registry_form_host_hint()}
				error={vm.errors.host ?? null}
			>
				{#snippet children(a11y)}
					<Input
						{...a11y}
						mono
						placeholder="ghcr.io"
						autocomplete="off"
						autocapitalize="none"
						spellcheck={false}
						value={vm.host}
						oninput={(event) => vm.setField('host', event.currentTarget.value)}
					/>
				{/snippet}
			</Field>
		{/if}
		<Field label={m.admin_registry_form_username()} error={vm.errors.username ?? null}>
			{#snippet children(a11y)}
				<Input
					{...a11y}
					autocomplete="off"
					autocapitalize="none"
					spellcheck={false}
					value={vm.username}
					oninput={(event) => vm.setField('username', event.currentTarget.value)}
				/>
			{/snippet}
		</Field>
		<Field
			label={m.admin_registry_form_password()}
			hint={vm.dialog?.mode === 'edit' ? m.admin_registry_form_password_hint_edit() : null}
			error={vm.errors.password ?? null}
		>
			{#snippet children(a11y)}
				<Input
					{...a11y}
					type="password"
					autocomplete="new-password"
					value={vm.password}
					oninput={(event) => vm.setField('password', event.currentTarget.value)}
				/>
			{/snippet}
		</Field>
		<Banner
			tone="info"
			title={m.admin_registry_scope_title()}
			body={m.admin_registry_scope_body()}
		/>
	</form>
	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={vm.isSaving} onclick={vm.closeDialog}>
			{m.common_cancel()}
		</Button>
		<Button type="submit" form={formId} size="sm" loading={vm.isSaving}>
			{m.common_save()}
		</Button>
	{/snippet}
</Modal>

<ConfirmDialog
	open={vm.removing !== null}
	title={m.admin_registry_delete_title({ host: vm.removing ?? '' })}
	body={vm.removeBody}
	confirmLabel={m.admin_registries_delete()}
	cancelLabel={m.common_cancel()}
	loading={vm.isRemoving}
	error={vm.removeError}
	onConfirm={vm.confirmRemove}
	onCancel={vm.cancelRemove}
/>
