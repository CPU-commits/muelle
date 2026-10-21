<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Banner,
		Button,
		Card,
		ConfirmDialog,
		Field,
		Input,
		LinkButton,
		Modal,
		Mono,
		StatusDot,
	} from '@/shared/ui'
	import type { ProxySettings } from '../../../domain'
	import { ProxySettingsViewModel } from './ProxySettings.viewModel.svelte.ts'
	import {
		fields,
		portFields,
		readonlyField,
		readonlyLabel,
		readonlyValue,
		stack,
		status,
		statusLabel,
		statusMeta,
		warning,
	} from './styles.css'

	// Las tarjetas de "Proxy y DNS" (`Ajustes.dc.html`). Solo se montan si el backend ya
	// tiene la Fase 3; si no, la pantalla muestra <ComingSoon>.
	let { proxy, id }: { proxy: ProxySettings; id?: string } = $props()

	const vm = new ProxySettingsViewModel(() => proxy)
	const uid = $props.id()
	const ipForm = `${uid}-ip`
	const acmeForm = `${uid}-acme`
	const portsForm = `${uid}-ports`
	const cloudflareForm = `${uid}-cf`
	const domainLabel = `${uid}-domain`
</script>

<div {id} class={stack}>
	<Card
		title={m.admin_cf_title()}
		description={m.admin_cf_description()}
		note={m.admin_cf_note()}
	>
		<div class={status}>
			{#if vm.proxy.cloudflare.tokenSet}
				<span class={statusLabel}><StatusDot status="ok" />{m.admin_cf_set()}</span>
				<span class={statusMeta}>
					{vm.cloudflareMeta}
				</span>
			{:else}
				<span class={statusLabel}><StatusDot status="idle" />{m.admin_cf_unset()}</span>
				<span class={statusMeta}>{m.admin_cf_unset_hint()}</span>
			{/if}
		</div>
		{#snippet footer()}
			{#if vm.proxy.cloudflare.tokenSet}
				<Button variant="ghost" size="sm" onclick={vm.askRemoveCloudflare}>
					{m.admin_cf_remove()}
				</Button>
			{/if}
			<Button variant="secondary" size="sm" onclick={vm.openCloudflare}>
				{vm.proxy.cloudflare.tokenSet ? m.admin_cf_replace() : m.admin_cf_add()}
			</Button>
		{/snippet}
	</Card>

	<Card
		title={m.admin_ip_title()}
		description={m.admin_ip_description()}
		note={m.admin_ip_note()}
	>
		<form
			id={ipForm}
			class={fields}
			novalidate
			onsubmit={(event) => {
				event.preventDefault()
				void vm.saveIp()
			}}
		>
			<Field
				label={m.admin_ip_v4()}
				hint={m.admin_ip_v4_hint()}
				error={vm.errors.ipv4 ?? null}
			>
				{#snippet children(a11y)}
					<Input
						{...a11y}
						mono
						inputmode="decimal"
						autocomplete="off"
						spellcheck={false}
						value={vm.ipv4}
						oninput={(event) => vm.setField('ipv4', event.currentTarget.value)}
					/>
				{/snippet}
			</Field>
			<Field label={m.admin_ip_v6()} optional error={vm.errors.ipv6 ?? null}>
				{#snippet children(a11y)}
					<Input
						{...a11y}
						mono
						autocomplete="off"
						spellcheck={false}
						value={vm.ipv6}
						oninput={(event) => vm.setField('ipv6', event.currentTarget.value)}
					/>
				{/snippet}
			</Field>
		</form>
		{#snippet footer()}
			<Button
				type="submit"
				form={ipForm}
				size="sm"
				loading={vm.isSavingIp}
				disabled={!vm.isIpDirty}
			>
				{m.common_save()}
			</Button>
		{/snippet}
	</Card>

	<Card
		title={m.admin_cert_title()}
		description={m.admin_cert_description()}
		note={m.admin_cert_note()}
	>
		<form
			id={acmeForm}
			class={fields}
			novalidate
			onsubmit={(event) => {
				event.preventDefault()
				void vm.saveAcme()
			}}
		>
			<Field label={m.admin_cert_email()} optional error={vm.errors.acmeEmail ?? null}>
				{#snippet children(a11y)}
					<Input
						{...a11y}
						type="email"
						autocomplete="email"
						value={vm.acmeEmail}
						oninput={(event) => vm.setField('acmeEmail', event.currentTarget.value)}
					/>
				{/snippet}
			</Field>
			<div class={readonlyField} role="group" aria-labelledby={domainLabel}>
				<span id={domainLabel} class={readonlyLabel}>{m.admin_cert_panel_domain()}</span>
				<span class={readonlyValue}><Mono size={13}>{vm.proxy.panelDomain}</Mono></span>
			</div>
		</form>
		{#snippet footer()}
			<Button
				type="submit"
				form={acmeForm}
				size="sm"
				loading={vm.isSavingAcme}
				disabled={!vm.isAcmeDirty}
			>
				{m.common_save()}
			</Button>
		{/snippet}
	</Card>

	<Card title={m.admin_ports_title()} description={m.admin_ports_description()}>
		{#if vm.movedTo}
			<!-- Queda hasta que se cierra, también al recargar. No pide tocar la configuración
			     de muelle: el origen permitido sigue al puerto HTTPS (contrato §0.1). -->
			<Banner
				tone="info"
				title={m.admin_ports_moved_title({ origin: vm.movedTo })}
				body={vm.movedBody}
			>
				{#snippet action()}
					{#if vm.canGoToMovedPanel}
						<LinkButton href={vm.movedPanelHref} variant="secondary" size="sm">
							{m.admin_ports_moved_action()}
						</LinkButton>
					{/if}
					<Button variant="ghost" size="sm" onclick={vm.dismissMove}>
						{m.admin_ports_moved_dismiss()}
					</Button>
				{/snippet}
			</Banner>
		{/if}
		{#if vm.rollback}
			<!-- muelle-caddy no arrancó con los puertos nuevos y muelle volvió a los de antes
			     (contrato §3.5). Si el panel no está en esa dirección, la ofrece. Cerrado, el
			     aviso de esa vuelta no vuelve a salir. -->
			<Banner tone="warn" title={vm.rollbackTitle} body={vm.rollbackBody}>
				{#snippet action()}
					{#if vm.rollbackHref}
						<LinkButton href={vm.rollbackHref} variant="secondary" size="sm">
							{m.admin_ports_rollback_action()}
						</LinkButton>
					{/if}
					<Button variant="ghost" size="sm" onclick={vm.dismissRollback}>
						{m.admin_ports_rollback_dismiss()}
					</Button>
				{/snippet}
			</Banner>
		{:else if vm.recreation?.kind === 'recreating'}
			<Banner
				tone="info"
				title={m.admin_ports_recreating_title()}
				body={m.admin_ports_recreating_body()}
			/>
		{:else if vm.recreation?.kind === 'done'}
			<Banner tone="info" dot="ok" title={m.admin_ports_recreated()} />
		{:else if vm.recreation?.kind === 'failed'}
			<Banner
				tone="warn"
				title={m.admin_ports_recreate_failed_title()}
				body={vm.recreation.error
					? m.admin_ports_recreate_failed_body({ error: vm.recreation.error })
					: m.admin_ports_recreate_failed_generic()}
			/>
		{:else if vm.recreation?.kind === 'unknown'}
			<!-- Pasaron 2 min sin una respuesta de GET /proxy que diga cómo terminó. -->
			<Banner
				tone="warn"
				title={m.admin_ports_recreate_unknown_title()}
				body={m.admin_ports_recreate_unknown_body()}
			/>
		{/if}
		<form
			id={portsForm}
			class={portFields}
			novalidate
			onsubmit={(event) => {
				event.preventDefault()
				vm.askSavePorts()
			}}
		>
			<Field label={m.admin_ports_http()} error={vm.errors.httpPort ?? null}>
				{#snippet children(a11y)}
					<Input
						{...a11y}
						mono
						type="number"
						min="1"
						max="65535"
						value={vm.httpPort}
						oninput={(event) => vm.setField('httpPort', event.currentTarget.value)}
					/>
				{/snippet}
			</Field>
			<Field label={m.admin_ports_https()} error={vm.errors.httpsPort ?? null}>
				{#snippet children(a11y)}
					<Input
						{...a11y}
						mono
						type="number"
						min="1"
						max="65535"
						value={vm.httpsPort}
						oninput={(event) => vm.setField('httpsPort', event.currentTarget.value)}
					/>
				{/snippet}
			</Field>
		</form>
		{#snippet footer()}
			<span class={warning}
				><StatusDot status="warn" size={6} />{m.admin_ports_warning()}</span
			>
			<Button
				type="submit"
				form={portsForm}
				size="sm"
				loading={vm.isSavingPorts}
				disabled={!vm.arePortsDirty}
			>
				{m.common_save()}
			</Button>
		{/snippet}
	</Card>
</div>

<Modal
	open={vm.cloudflareOpen}
	title={m.admin_cf_modal_title()}
	description={m.admin_cf_modal_description()}
	onClose={vm.closeCloudflare}
>
	{#if vm.cloudflareError}
		<Banner
			tone="error"
			title={vm.cloudflareError.title}
			{...vm.cloudflareError.body ? { body: vm.cloudflareError.body } : {}}
		/>
	{/if}
	<form
		id={cloudflareForm}
		novalidate
		onsubmit={(event) => {
			event.preventDefault()
			void vm.saveCloudflare()
		}}
	>
		<Field
			label={m.admin_cf_token()}
			hint={m.admin_cf_token_hint()}
			error={vm.errors.cloudflareToken ?? null}
		>
			{#snippet children(a11y)}
				<Input
					{...a11y}
					type="password"
					mono
					autocomplete="new-password"
					spellcheck={false}
					value={vm.cloudflareToken}
					oninput={(event) => vm.setField('cloudflareToken', event.currentTarget.value)}
				/>
			{/snippet}
		</Field>
	</form>
	{#snippet footer()}
		<Button
			variant="secondary"
			size="sm"
			disabled={vm.isSavingCloudflare}
			onclick={vm.closeCloudflare}
		>
			{m.common_cancel()}
		</Button>
		<Button type="submit" form={cloudflareForm} size="sm" loading={vm.isSavingCloudflare}>
			{m.common_save()}
		</Button>
	{/snippet}
</Modal>

<ConfirmDialog
	open={vm.confirmingPorts}
	tone="primary"
	title={m.admin_ports_confirm_title()}
	body={m.admin_ports_confirm_body({ http: vm.httpPort.trim(), https: vm.httpsPort.trim() })}
	confirmLabel={m.admin_ports_confirm()}
	cancelLabel={m.common_cancel()}
	loading={vm.isSavingPorts}
	error={vm.portsError}
	onConfirm={vm.savePorts}
	onCancel={vm.cancelPorts}
>
	{#if vm.isSavingPorts}
		<Banner tone="info" title={m.admin_ports_applying()} body={m.admin_ports_applying_body()} />
	{:else if vm.portsMove}
		<Banner
			tone="warn"
			title={m.admin_ports_move_title({ origin: vm.portsMove.origin })}
			body={m.admin_ports_move_body({ port: vm.portsMove.port })}
		/>
	{/if}
</ConfirmDialog>

<ConfirmDialog
	open={vm.confirmingRemoveCloudflare}
	tone="danger"
	title={m.admin_cf_remove_title()}
	body={m.admin_cf_remove_body()}
	confirmLabel={m.admin_cf_remove()}
	cancelLabel={m.common_cancel()}
	loading={vm.isRemovingCloudflare}
	error={vm.removeCloudflareError}
	onConfirm={vm.removeCloudflare}
	onCancel={vm.cancelRemoveCloudflare}
/>
