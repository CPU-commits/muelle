<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Banner,
		Button,
		Card,
		ConfirmDialog,
		Field,
		LinkButton,
		Switch,
		Textarea,
	} from '@/shared/ui'
	import type { Settings } from '../../../domain'
	import { ServerCommand } from '../../components/serverCommand'
	import { SecuritySettingsViewModel } from './SecuritySettings.viewModel.svelte.ts'
	import {
		cidrs,
		clientIp,
		clientIpOut,
		recovery,
		switchControl,
		switchHint,
		switchRow,
		switchText,
		switchTitle,
	} from './styles.css'

	// La tarjeta "Seguridad" de Ajustes (`Ajustes.dc.html`).
	let { settings }: { settings: Settings } = $props()

	const vm = new SecuritySettingsViewModel(() => settings)
</script>

<Card title={m.admin_security_title()} description={m.admin_security_description()}>
	{#if vm.totpRequiredFirst}
		<Banner
			tone="warn"
			title={m.admin_security_totp_first()}
			body={m.admin_security_totp_first_body()}
		>
			{#snippet action()}
				<LinkButton href={resolve('/2fa')} variant="secondary" size="sm">
					{m.admin_security_totp_first_action()}
				</LinkButton>
			{/snippet}
		</Banner>
	{/if}

	<label class={switchRow}>
		<span class={switchText}>
			<span class={switchTitle}>{m.admin_security_2fa()}</span>
			<span class={switchHint}>{m.admin_security_2fa_hint()}</span>
		</span>
		<Switch
			class={switchControl}
			checked={vm.require2fa}
			disabled={vm.isSaving}
			onchange={(event) => vm.setRequire2fa(event.currentTarget.checked)}
		/>
	</label>

	<div class={cidrs}>
		<Field
			label={m.admin_security_cidrs()}
			hint={m.admin_security_cidrs_hint()}
			error={vm.cidrsError}
		>
			{#snippet children(a11y)}
				<Textarea
					{...a11y}
					mono
					rows={3}
					spellcheck={false}
					value={vm.cidrs}
					oninput={(event) => vm.setCidrs(event.currentTarget.value)}
				/>
			{/snippet}
		</Field>
		{#if vm.clientIpHint}
			<p class={[clientIp, vm.clientIpAllowed === false && clientIpOut]} aria-live="polite">
				{vm.clientIpHint}
			</p>
		{/if}
	</div>

	{#snippet footer()}
		<Button size="sm" loading={vm.isSaving} disabled={!vm.isDirty} onclick={vm.save}>
			{m.common_save()}
		</Button>
	{/snippet}
</Card>

<ConfirmDialog
	open={vm.confirmingLockout}
	tone="danger"
	title={m.admin_security_lockout_title()}
	body={vm.lockoutBody}
	confirmLabel={m.admin_security_lockout_confirm()}
	cancelLabel={m.common_cancel()}
	loading={vm.isSaving}
	error={vm.lockoutError}
	onConfirm={vm.confirmLockout}
	onCancel={vm.cancelLockout}
>
	<ServerCommand
		class={recovery}
		label={m.admin_security_recovery_label()}
		command={vm.recoveryCommand}
		copyLabel={m.admin_security_recovery_copy()}
	/>
</ConfirmDialog>
