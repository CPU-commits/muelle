<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Banner,
		Button,
		ComingSoon,
		ConfirmDialog,
		EmptyState,
		ErrorState,
		Field,
		Input,
		LoadingState,
		Modal,
		PageHeader,
		srOnly,
		Switch,
	} from '@/shared/ui'
	import { DomainCard } from '../../components/domainCard'
	import { type DomainsProps, DomainsViewModel } from './Domains.viewModel.svelte.ts'
	import {
		editForm,
		fieldError,
		footnote,
		form,
		formRow,
		hostnameField,
		list,
		page,
		proxiedField,
		targetField,
	} from './styles.css'

	let props: DomainsProps = $props()

	const vm = new DomainsViewModel(() => props)
	const id = $props.id()
</script>

<div class={page}>
	<!-- Una sola región para los dominios con error (no una por tarjeta). Está desde el
	     principio, así se anuncia cuando llega la lista o cambia cuáles fallan. -->
	<p class={srOnly} role="alert">{vm.failingAnnouncement}</p>
	<PageHeader title={m.nav_app_domains()} subtitle={m.ingress_subtitle()} />

	{#if vm.pendingPhase !== null}
		<ComingSoon phase={vm.pendingPhase} feature={m.ingress_coming()} />
	{:else if vm.isLoading}
		<LoadingState label={m.ingress_loading()} />
	{:else if vm.loadError}
		<ErrorState
			title={m.ingress_error_title()}
			body={m.common_unavailable()}
			retryLabel={m.common_retry()}
			onRetry={vm.retryLoad}
		/>
	{:else}
		<form
			class={form}
			aria-label={m.ingress_add_label()}
			novalidate
			onsubmit={(event) => {
				event.preventDefault()
				void vm.add()
			}}
		>
			{#if vm.formError}
				<Banner tone="error" title={vm.formError} />
			{/if}
			<div class={formRow}>
				<div class={hostnameField}>
					<Input
						mono
						name="hostname"
						aria-label={m.ingress_hostname()}
						aria-invalid={vm.hostnameError !== null}
						aria-describedby={vm.hostnameError ? `${id}-hostname-error` : undefined}
						placeholder={m.ingress_hostname_placeholder()}
						autocomplete="off"
						autocapitalize="none"
						spellcheck={false}
						value={vm.hostname}
						disabled={vm.isAdding}
						oninput={(event) => vm.setHostname(event.currentTarget.value)}
					/>
					{#if vm.hostnameError}
						<p id={`${id}-hostname-error`} class={fieldError}>{vm.hostnameError}</p>
					{/if}
				</div>
				<div class={targetField}>
					<Input
						mono
						name="target"
						list={`${id}-targets`}
						aria-label={m.ingress_target()}
						aria-invalid={vm.targetError !== null}
						aria-describedby={vm.targetError ? `${id}-target-error` : undefined}
						placeholder={vm.targetPlaceholder}
						autocomplete="off"
						autocapitalize="none"
						spellcheck={false}
						value={vm.targetValue}
						disabled={vm.isAdding}
						oninput={(event) => vm.setTarget(event.currentTarget.value)}
					/>
					<datalist id={`${id}-targets`}>
						{#each vm.suggestions as suggestion (suggestion)}
							<option value={suggestion}></option>
						{/each}
					</datalist>
					{#if vm.targetError}
						<p id={`${id}-target-error`} class={fieldError}>{vm.targetError}</p>
					{/if}
				</div>
				<label class={proxiedField}>
					<Switch
						checked={vm.proxied}
						disabled={vm.isAdding}
						onchange={(event) => vm.setProxied(event.currentTarget.checked)}
					/>
					<span>{m.ingress_proxied()}</span>
				</label>
				<Button type="submit" loading={vm.isAdding}>{m.ingress_add()}</Button>
			</div>
		</form>

		{#if vm.isEmpty}
			<EmptyState
				icon="globe"
				title={m.ingress_empty_title()}
				body={m.ingress_empty_body()}
			/>
		{:else}
			<ul class={list} aria-label={m.ingress_list_label()}>
				{#each vm.list as domain (domain.hostname)}
					<li>
						<DomainCard
							{domain}
							busy={vm.isBusy(domain)}
							onToggleProxied={(proxied) => void vm.toggleProxied(domain, proxied)}
							onEdit={() => vm.openEdit(domain)}
							onAdopt={() => void vm.adopt(domain)}
							onRetry={() => void vm.retry(domain)}
							onRemove={() => vm.askRemove(domain)}
						/>
					</li>
				{/each}
			</ul>
		{/if}

		<p class={footnote}>{m.ingress_footnote()}</p>
	{/if}
</div>

<Modal
	open={vm.editing !== null}
	title={m.ingress_edit_title({ hostname: vm.editing?.hostname ?? '' })}
	description={m.ingress_edit_body()}
	onClose={vm.closeEdit}
>
	<form
		id={`${id}-edit`}
		class={editForm}
		novalidate
		onsubmit={(event) => {
			event.preventDefault()
			void vm.saveEdit()
		}}
	>
		<Field label={m.ingress_edit_target()} hint={m.ingress_edit_hint()} error={vm.editError}>
			{#snippet children(a11y)}
				<Input
					{...a11y}
					mono
					list={`${id}-targets`}
					autocomplete="off"
					autocapitalize="none"
					spellcheck={false}
					value={vm.editTarget}
					disabled={vm.isSavingEdit}
					oninput={(event) => vm.setEditTarget(event.currentTarget.value)}
				/>
			{/snippet}
		</Field>
	</form>
	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={vm.isSavingEdit} onclick={vm.closeEdit}>
			{m.common_cancel()}
		</Button>
		<Button type="submit" form={`${id}-edit`} size="sm" loading={vm.isSavingEdit}>
			{m.common_save()}
		</Button>
	{/snippet}
</Modal>

<ConfirmDialog
	open={vm.removing !== null}
	title={m.ingress_remove_title({ hostname: vm.removing?.hostname ?? '' })}
	body={m.ingress_remove_body()}
	confirmLabel={m.common_delete()}
	cancelLabel={m.common_cancel()}
	loading={vm.isRemoving}
	error={vm.removeError}
	onConfirm={() => void vm.confirmRemove()}
	onCancel={vm.cancelRemove}
/>
