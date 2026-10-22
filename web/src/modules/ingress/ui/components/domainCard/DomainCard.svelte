<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Button,
		CopyButton,
		Icon,
		Menu,
		type MenuEntry,
		Mono,
		srOnly,
		StatusDot,
		Switch,
		Table,
	} from '@/shared/ui'
	import {
		type AppDomain,
		canToggleProxied,
		domainState,
		formatTarget,
		isReachable,
		manualRecords,
		originCaNotice,
		publicAddress,
	} from '../../../domain'
	import {
		certificateText,
		dnsText,
		originCaFailureText,
		originCaHint,
		stateLabel,
		stateTone,
	} from '../labels/domainLabels'
	import {
		actions,
		card,
		copyCell,
		errorText,
		footer,
		footerStack,
		footerText,
		info,
		link,
		linkIcon,
		main,
		manualText,
		meta,
		originHint,
		proxied,
		recordsBox,
		state as stateClass,
		stateTones,
		title,
		titleRow,
		warnText,
	} from './styles.css'

	// Un dominio de App-Dominios.dc.html: hostname, estado, destino, DNS y certificado, con
	// la acción que corresponde al estado. Solo markup: las acciones las hace la pantalla.
	type Props = {
		domain: AppDomain
		/** Hay una acción en curso sobre este dominio. */
		busy: boolean
		onToggleProxied: (proxied: boolean) => void
		onEdit: () => void
		onAdopt: () => void
		onRetry: () => void
		onRemove: () => void
	}

	let { domain, busy, onToggleProxied, onEdit, onAdopt, onRetry, onRemove }: Props = $props()

	const titleId = $props.id()
	const current = $derived(domainState(domain))
	const tone = $derived(stateTone(current))
	const records = $derived(manualRecords(domain))
	const certificate = $derived(certificateText(domain))
	const ip = $derived(publicAddress(domain))
	// El certificado de Cloudflare (Origin CA, contrato §3.7): cuál se sirve y qué avisar.
	const originCa = $derived(originCaNotice(domain))
	const originCaHintText = $derived(originCaHint(originCa))
	const servesOriginCa = $derived(domain.certSource === 'cloudflare_origin_ca')
	const originCaFailed = $derived(originCa === 'failed' || originCa === 'renew_failed')

	const menuItems: MenuEntry[] = $derived([
		...(current === 'waiting_dns'
			? [
					{
						kind: 'action' as const,
						id: 'edit',
						label: m.ingress_menu_edit(),
						onSelect: onEdit,
					},
				]
			: []),
		...(current === 'active' || current === 'provisioning'
			? [
					{
						kind: 'action' as const,
						id: 'retry',
						label: m.ingress_menu_retry(),
						icon: 'refresh' as const,
						onSelect: onRetry,
					},
				]
			: []),
		// El registro ajeno puede ya no estar: muelle vuelve a mirar y, si no está, lo crea.
		...(current === 'record_exists'
			? [
					{
						kind: 'action' as const,
						id: 'recheck',
						label: m.ingress_menu_recheck(),
						icon: 'refresh' as const,
						onSelect: onRetry,
					},
				]
			: []),
		{ kind: 'separator', id: 'separator' },
		{
			kind: 'action',
			id: 'remove',
			label: m.common_delete(),
			icon: 'trash',
			tone: 'danger',
			onSelect: onRemove,
		},
	])
</script>

<article class={card} aria-labelledby={titleId}>
	<div class={main}>
		<div class={info}>
			<div class={titleRow}>
				<h2 id={titleId} class={title}>
					{#if isReachable(domain)}
						<a
							class={link}
							href={`https://${domain.hostname}`}
							target="_blank"
							rel="noopener noreferrer"
						>
							{domain.hostname}
							<span class={linkIcon}><Icon name="arrowUpRight" size={13} /></span>
							<span class={srOnly}>{m.ingress_open()}</span>
						</a>
					{:else}
						{domain.hostname}
					{/if}
				</h2>
				<span class={[stateClass, stateTones[tone]]}>
					<StatusDot status={tone} />
					{stateLabel(current)}
				</span>
			</div>
			<p class={meta}>
				<span>
					<span aria-hidden="true">→</span><span class={srOnly}
						>{m.ingress_points_to()}</span
					>
					<Mono tone="secondary">{formatTarget(domain.target)}</Mono>
				</span>
				<span>{dnsText(domain)}</span>
				{#if certificate && servesOriginCa}
					<!-- Los navegadores no confían en él: se entra por Cloudflare. -->
					<span title={m.ingress_origin_ca_trust()}>{certificate}</span>
					<span class={srOnly}>{m.ingress_origin_ca_trust()}</span>
				{:else if certificate}
					<span>{certificate}</span>
				{/if}
			</p>
			{#if originCaHintText}
				<p class={originHint}>{originCaHintText}</p>
			{/if}
		</div>

		<div class={actions}>
			{#if canToggleProxied(domain)}
				<label class={proxied}>
					<Switch
						checked={domain.proxied}
						disabled={busy}
						aria-label={m.ingress_proxied_label({ hostname: domain.hostname })}
						onchange={(event) => onToggleProxied(event.currentTarget.checked)}
					/>
					<span aria-hidden="true">{m.ingress_proxied()}</span>
				</label>
			{/if}
			{#if current === 'waiting_dns'}
				<Button variant="secondary" size="sm" loading={busy} onclick={onRetry}>
					<Icon name="refresh" size={14} />
					{m.ingress_verify()}
				</Button>
			{:else}
				<Button variant="secondary" size="sm" disabled={busy} onclick={onEdit}>
					{m.common_edit()}
				</Button>
			{/if}
			<Menu label={m.ingress_options({ hostname: domain.hostname })} items={menuItems} />
		</div>
	</div>

	{#if current === 'record_exists'}
		<div class={footer}>
			<p class={footerText}>
				{#if ip}
					{m.ingress_adopt_before()}
					<Mono tone="primary">{ip}</Mono>
					{m.ingress_adopt_after()}
				{:else}
					{m.ingress_adopt_body_no_ip()}
				{/if}
			</p>
			<Button size="sm" loading={busy} onclick={onAdopt}>{m.ingress_adopt()}</Button>
		</div>
	{:else if current === 'error'}
		<div class={footer}>
			<p class={[footerText, errorText]}>{domain.lastError ?? m.ingress_error_fallback()}</p>
			<Button variant="secondary" size="sm" loading={busy} onclick={onRetry}>
				<Icon name="refresh" size={14} />
				{m.common_retry()}
			</Button>
		</div>
	{:else if originCaFailed}
		<!-- La emisión (o la renovación) del de Cloudflare falló: el dominio sigue andando, con
		     Let's Encrypt o con el actual, y Reintentar reinicia la espera (contrato §3.2). -->
		<div class={footer}>
			<p class={[footerText, warnText]}>{originCaFailureText(domain)}</p>
			<Button variant="secondary" size="sm" loading={busy} onclick={onRetry}>
				<Icon name="refresh" size={14} />
				{m.common_retry()}
			</Button>
		</div>
	{:else if current === 'waiting_dns' && records.length > 0}
		<div class={[footer, footerStack]}>
			<p class={manualText}>{m.ingress_manual_body()}</p>
			<div class={recordsBox}>
				<Table
					framed={false}
					minWidth="520px"
					caption={m.ingress_records_caption({ hostname: domain.hostname })}
				>
					<thead>
						<tr>
							<th scope="col">{m.ingress_record_type()}</th>
							<th scope="col">{m.ingress_record_name()}</th>
							<th scope="col">{m.ingress_record_value()}</th>
							<th scope="col"
								><span class={srOnly}>{m.ingress_record_copy_column()}</span></th
							>
						</tr>
					</thead>
					<tbody>
						{#each records as record, index (`${record.type}-${record.content}`)}
							<tr>
								<td><Mono>{record.type}</Mono></td>
								<td><Mono>{record.name}</Mono></td>
								<td id={`${titleId}-record-${index}`}
									><Mono>{record.content}</Mono></td
								>
								<td class={copyCell}>
									<CopyButton
										value={record.content}
										target={`${titleId}-record-${index}`}
										label={m.ingress_record_copy()}
										iconOnly
										variant="ghost"
										size="xs"
									/>
								</td>
							</tr>
						{/each}
					</tbody>
				</Table>
			</div>
		</div>
	{/if}
</article>
