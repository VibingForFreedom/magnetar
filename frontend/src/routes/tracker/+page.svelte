<script lang="ts">
	import { metrics, trackerScrapeRateHistory } from '$lib/stores/metrics';
	import {
		fetchTrackerStats,
		triggerTrackerScrape,
		type TrackerStatsResponse,
		type ScrapeResponse
	} from '$lib/api';
	import { formatNumber, formatRate, formatBytes, formatTimestamp } from '$lib/utils';
	import StatCard from '$lib/components/StatCard.svelte';
	import RateGraph from '$lib/components/RateGraph.svelte';
	import {
		Radar,
		CheckCircle,
		XCircle,
		RefreshCw,
		Loader2,
		Zap,
		Copy,
		Check
	} from 'lucide-svelte';
	import { onMount } from 'svelte';

	let data = $state<TrackerStatsResponse | null>(null);
	let loading = $state(false);
	let scraping = $state(false);
	let scrapeTotal = $state<number | null>(null);
	let scrapeError = $state<string | null>(null);
	let copiedHash = $state<string | null>(null);

	async function loadData() {
		loading = true;
		try {
			data = await fetchTrackerStats(50);
		} finally {
			loading = false;
		}
	}

	// Detect background scrape activity from SSE metrics rate
	let scrapeActive = $derived(($metrics?.tracker_scrape_rate ?? 0) > 0);

	async function handleScrape() {
		if (scraping) return;
		scraping = true;
		scrapeTotal = null;
		scrapeError = null;
		try {
			const result = await triggerTrackerScrape();
			scrapeTotal = result.total;
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : String(e);
			if (msg.includes('409')) {
				scrapeError = 'Scrape already in progress';
			} else {
				scrapeError = msg;
			}
		} finally {
			scraping = false;
		}
	}

	function copyMagnet(hash: string) {
		const magnet = `magnet:?xt=urn:btih:${hash}`;
		navigator.clipboard.writeText(magnet);
		copiedHash = hash;
		setTimeout(() => {
			if (copiedHash === hash) copiedHash = null;
		}, 2000);
	}

	function protoBadgeClass(proto: string): string {
		return proto === 'udp'
			? 'bg-accent-blue/20 text-accent-blue'
			: 'bg-accent-purple/20 text-accent-purple';
	}

	onMount(() => {
		loadData();
		const interval = setInterval(loadData, 30000);
		return () => clearInterval(interval);
	});
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-xl font-semibold text-text-primary">Tracker</h1>
			<p class="mt-1 text-sm text-text-secondary">Per-tracker scrape state and recently updated torrents</p>
		</div>
		<div class="flex items-center gap-3">
			{#if scrapeActive}
				<span class="inline-flex items-center gap-1.5 rounded-full bg-accent-blue/20 px-2.5 py-1 text-xs font-medium text-accent-blue">
					<Loader2 class="h-3 w-3 animate-spin" />
					Scraping{scrapeTotal ? ` ${formatNumber(scrapeTotal)} torrents` : ''}...
				</span>
			{:else if scrapeTotal && !scrapeActive}
				<span class="text-xs text-accent-green">
					Scrape complete ({formatNumber(scrapeTotal)} torrents)
				</span>
			{/if}
			{#if scrapeError}
				<span class="text-xs text-accent-red">{scrapeError}</span>
			{/if}
			<button
				onclick={handleScrape}
				disabled={scraping || scrapeActive}
				class="inline-flex items-center gap-1.5 rounded-md bg-accent-blue px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-accent-blue/80 disabled:opacity-50"
			>
				{#if scraping}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{:else}
					<Zap class="h-3.5 w-3.5" />
				{/if}
				Trigger Scrape
			</button>
		</div>
	</div>

	<!-- Stats Row -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		<div class="rounded-lg border border-border bg-bg-secondary p-4">
			<div class="flex items-center justify-between">
				<span class="text-xs font-medium uppercase tracking-wider text-text-secondary">Scrape Rate</span>
				<Radar class="h-4 w-4 text-accent-blue" />
			</div>
			<div class="mt-2 text-2xl font-semibold tracking-tight text-text-primary">
				{$metrics ? formatRate($metrics.tracker_scrape_rate) : '--'}
			</div>
			<div class="mt-1 text-xs text-text-secondary">
				{$metrics ? formatNumber($metrics.tracker_scrape_attempts) : '0'} total attempts
			</div>
			<div class="mt-2">
				<RateGraph data={$trackerScrapeRateHistory} color="#6b8aaf" height={32} />
			</div>
		</div>
		<StatCard
			label="Successes"
			value={$metrics ? formatNumber($metrics.tracker_scrape_successes) : '--'}
			subtitle={$metrics && $metrics.tracker_scrape_attempts > 0
				? `${(($metrics.tracker_scrape_successes / $metrics.tracker_scrape_attempts) * 100).toFixed(1)}%`
				: undefined}
			icon={CheckCircle}
			color="text-accent-green"
		/>
		<StatCard
			label="Failures"
			value={$metrics ? formatNumber($metrics.tracker_scrape_failures) : '--'}
			icon={XCircle}
			color="text-accent-red"
		/>
		<StatCard
			label="Updated"
			value={$metrics ? formatNumber($metrics.tracker_scrape_updated) : '--'}
			icon={RefreshCw}
			color="text-accent-amber"
		/>
	</div>

	<!-- Tracker State Table -->
	<div class="rounded-lg border border-border bg-bg-secondary">
		<div class="border-b border-border px-4 py-3">
			<h2 class="text-sm font-medium text-text-primary">Tracker State</h2>
		</div>

		{#if loading && !data}
			<div class="flex items-center justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin text-text-secondary" />
			</div>
		{:else if data && data.trackers.length > 0}
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-border text-left text-xs uppercase tracking-wider text-text-secondary">
							<th class="px-4 py-3 font-medium">Host</th>
							<th class="px-4 py-3 font-medium">Protocol</th>
							<th class="px-4 py-3 font-medium">Batch Limit</th>
							<th class="px-4 py-3 font-medium">Max Limit</th>
							<th class="px-4 py-3 font-medium">Successes</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-border">
						{#each data.trackers as t (t.host)}
							<tr class="hover:bg-bg-tertiary/30">
								<td class="px-4 py-3 font-mono text-xs text-text-primary">{t.host}</td>
								<td class="px-4 py-3">
									<span
										class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium uppercase {protoBadgeClass(t.protocol)}"
									>
										{t.protocol}
									</span>
								</td>
								<td class="px-4 py-3">
									<span class="text-text-primary">{t.batch_limit}</span>
									{#if t.batch_limit < t.initial_limit}
										<span class="ml-1.5 text-xs text-accent-amber">reduced</span>
									{/if}
								</td>
								<td class="px-4 py-3 text-text-secondary">{t.initial_limit}</td>
								<td class="px-4 py-3 text-text-secondary">{formatNumber(t.success_count)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else}
			<div class="py-8 text-center text-sm text-text-secondary">No trackers configured</div>
		{/if}
	</div>

	<!-- Recently Updated Table -->
	<div class="rounded-lg border border-border bg-bg-secondary">
		<div class="border-b border-border px-4 py-3">
			<h2 class="text-sm font-medium text-text-primary">Recently Updated</h2>
		</div>

		{#if loading && !data}
			<div class="flex items-center justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin text-text-secondary" />
			</div>
		{:else if data && data.recently_updated.length > 0}
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-border text-left text-xs uppercase tracking-wider text-text-secondary">
							<th class="px-4 py-3 font-medium">Name</th>
							<th class="px-4 py-3 font-medium">Size</th>
							<th class="px-4 py-3 font-medium">Category</th>
							<th class="px-4 py-3 font-medium">Seeders</th>
							<th class="px-4 py-3 font-medium">Leechers</th>
							<th class="px-4 py-3 font-medium">Updated At</th>
							<th class="px-4 py-3 font-medium w-10"></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-border">
						{#each data.recently_updated as t (t.info_hash)}
							<tr class="hover:bg-bg-tertiary/30">
								<td class="max-w-xs truncate px-4 py-3 text-text-primary" title={t.name}>
									{t.name}
								</td>
								<td class="whitespace-nowrap px-4 py-3 text-text-secondary">{formatBytes(t.size)}</td>
								<td class="px-4 py-3">
									<span class="inline-flex rounded-full bg-bg-tertiary px-2 py-0.5 text-xs font-medium capitalize text-text-secondary">
										{t.category}
									</span>
								</td>
								<td class="px-4 py-3 text-accent-green">{formatNumber(t.seeders)}</td>
								<td class="px-4 py-3 text-accent-red">{formatNumber(t.leechers)}</td>
								<td class="whitespace-nowrap px-4 py-3 text-text-secondary">
									{formatTimestamp(t.updated_at ?? 0)}
								</td>
								<td class="px-4 py-3">
									<button
										onclick={() => copyMagnet(t.info_hash)}
										class="rounded p-1 text-text-secondary transition-colors hover:bg-bg-tertiary hover:text-text-primary"
										title="Copy magnet link"
									>
										{#if copiedHash === t.info_hash}
											<Check class="h-3.5 w-3.5 text-accent-green" />
										{:else}
											<Copy class="h-3.5 w-3.5" />
										{/if}
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else}
			<div class="py-8 text-center text-sm text-text-secondary">No recently updated torrents</div>
		{/if}
	</div>
</div>
