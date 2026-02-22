<script lang="ts">
	import { metrics, rateHistory, metadataRateHistory, matchRateHistory, trackerScrapeRateHistory } from '$lib/stores/metrics';
	import { fetchStats, type Stats } from '$lib/api';
	import { formatNumber, formatRate, formatUptime, formatBytes } from '$lib/utils';
	import StatCard from '$lib/components/StatCard.svelte';
	import RateGraph from '$lib/components/RateGraph.svelte';
	import { Database, Radio, Zap, Target, HardDrive, Radar } from 'lucide-svelte';
	import { onMount } from 'svelte';

	let stats = $state<Stats | null>(null);

	onMount(() => {
		fetchStats().then((s) => (stats = s));
		const interval = setInterval(() => {
			fetchStats().then((s) => (stats = s));
		}, 10000);
		return () => clearInterval(interval);
	});
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-xl font-semibold text-text-primary">Dashboard</h1>
		<p class="mt-1 text-sm text-text-secondary">Real-time crawler and matcher metrics</p>
	</div>

	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
		<StatCard
			label="Total Torrents"
			value={stats ? formatNumber(stats.total_torrents) : '--'}
			subtitle={stats ? `${formatNumber(stats.matched)} matched` : undefined}
			icon={Database}
			color="text-accent-blue"
		/>
		<StatCard
			label="Discovery Rate"
			value={$metrics ? formatRate($metrics.discovery_rate) : '--'}
			subtitle={$metrics ? `${formatNumber($metrics.torrents_discovered)} total` : undefined}
			icon={Radio}
			color="text-accent-green"
		/>
		<StatCard
			label="Metadata Rate"
			value={$metrics ? formatRate($metrics.metadata_rate) : '--'}
			subtitle={$metrics ? `${formatNumber($metrics.metadata_fetched)} fetched` : undefined}
			icon={Zap}
			color="text-accent-amber"
		/>
		<StatCard
			label="Match Rate"
			value={$metrics ? formatRate($metrics.match_rate) : '--'}
			subtitle={$metrics ? `${formatNumber($metrics.match_successes)} / ${formatNumber($metrics.match_attempts)} attempts` : undefined}
			icon={Target}
			color="text-accent-purple"
		/>
		<StatCard
			label="Tracker Scrape"
			value={$metrics ? formatRate($metrics.tracker_scrape_rate) : '--'}
			subtitle={$metrics ? `${formatNumber($metrics.tracker_scrape_successes)} / ${formatNumber($metrics.tracker_scrape_attempts)} attempts` : undefined}
			icon={Radar}
			color="text-accent-red"
		/>
		<StatCard
			label="DHT Nodes"
			value={$metrics ? formatNumber($metrics.dht_nodes_visited) : '--'}
			subtitle={$metrics ? `${formatNumber($metrics.dht_info_hashes_recv)} hashes` : undefined}
			icon={HardDrive}
			color="text-accent-blue"
		/>
	</div>

	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		<div class="flex flex-col justify-center rounded-lg border border-border bg-bg-secondary p-3 text-center">
			<div class="text-xs font-medium uppercase tracking-wider text-text-secondary">Discovery Rate (60s)</div>
			<div class="mb-1 text-2xl font-semibold tracking-tight text-text-primary">{$metrics ? `${formatNumber($metrics.torrents_discovered)} total` : '--'}</div>
			<RateGraph data={$rateHistory} color="#6ba38a" height={32} />
		</div>
		<div class="flex flex-col justify-center rounded-lg border border-border bg-bg-secondary p-3 text-center">
			<div class="text-xs font-medium uppercase tracking-wider text-text-secondary">Metadata Rate (60s)</div>
			<div class="mb-1 text-2xl font-semibold tracking-tight text-text-primary">{$metrics ? `${formatNumber($metrics.metadata_fetched)} fetched` : '--'}</div>
			<RateGraph data={$metadataRateHistory} color="#af9a6b" height={32} />
		</div>
		<div class="flex flex-col justify-center rounded-lg border border-border bg-bg-secondary p-3 text-center">
			<div class="text-xs font-medium uppercase tracking-wider text-text-secondary">Match Rate (60s)</div>
			<div class="mb-1 text-2xl font-semibold tracking-tight text-text-primary">{$metrics ? `${formatNumber($metrics.match_successes)} / ${formatNumber($metrics.match_attempts)} attempts` : '--'}</div>
			<RateGraph data={$matchRateHistory} color="#8a6baf" height={32} />
		</div>
		<div class="flex flex-col justify-center rounded-lg border border-border bg-bg-secondary p-3 text-center">
			<div class="text-xs font-medium uppercase tracking-wider text-text-secondary">Tracker Scrape Rate (60s)</div>
			<div class="mb-1 text-2xl font-semibold tracking-tight text-text-primary">{$metrics ? `${formatNumber($metrics.tracker_scrape_successes)} / ${formatNumber($metrics.tracker_scrape_attempts)} scraped` : '--'}</div>
			<RateGraph data={$trackerScrapeRateHistory} color="#af6b6b" height={32} />
		</div>
	</div>

	{#if stats}
		<div class="rounded-lg border border-border bg-bg-secondary p-4">
			<div class="mb-3 text-xs font-medium uppercase tracking-wider text-text-secondary">Database Breakdown</div>
			<div class="flex flex-wrap gap-8 text-sm">
				<div>
					<span class="text-text-secondary">Unmatched:</span>
					<span class="ml-1 font-medium text-accent-amber">{formatNumber(stats.unmatched)}</span>
				</div>
				<div>
					<span class="text-text-secondary">Matched:</span>
					<span class="ml-1 font-medium text-accent-green">{formatNumber(stats.matched)}</span>
				</div>
				<div>
					<span class="text-text-secondary">Failed:</span>
					<span class="ml-1 font-medium text-accent-red">{formatNumber(stats.failed)}</span>
				</div>
				<div>
					<span class="text-text-secondary">DB Size:</span>
					<span class="ml-1 font-medium text-text-primary">{formatBytes(stats.db_size)}</span>
				</div>
				{#if $metrics}
					<div>
						<span class="text-text-secondary">Uptime:</span>
						<span class="ml-1 font-medium text-text-primary">{formatUptime($metrics.uptime_seconds)}</span>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
