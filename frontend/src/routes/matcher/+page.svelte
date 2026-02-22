<script lang="ts">
	import { metrics, matchRateHistory } from '$lib/stores/metrics';
	import {
		fetchStats,
		fetchMatcherRecent,
		fetchMatcherFailures,
		toggleMatcher,
		triggerMatcher,
		rematch,
		type Stats,
		type MatcherListResponse
	} from '$lib/api';
	import { formatNumber, formatRate } from '$lib/utils';
	import StatCard from '$lib/components/StatCard.svelte';
	import RateGraph from '$lib/components/RateGraph.svelte';
	import MatcherList from '$lib/components/MatcherList.svelte';
	import Pagination from '$lib/components/Pagination.svelte';
	import { Target, CheckCircle, AlertTriangle, Clock, Loader2, Play, Pause, RotateCcw, Zap } from 'lucide-svelte';
	import { onMount } from 'svelte';

	let stats = $state<Stats | null>(null);
	let tab = $state<'recent' | 'failures'>('recent');
	let recentData = $state<MatcherListResponse | null>(null);
	let failureData = $state<MatcherListResponse | null>(null);
	let recentPage = $state(1);
	let failurePage = $state(1);
	let loading = $state(false);
	let triggering = $state(false);
	let resetting = $state(false);
	let toggling = $state(false);

	async function loadStats() {
		stats = await fetchStats();
	}

	async function loadRecent() {
		loading = true;
		try {
			recentData = await fetchMatcherRecent(recentPage, 20);
		} finally {
			loading = false;
		}
	}

	async function loadFailures() {
		loading = true;
		try {
			failureData = await fetchMatcherFailures(failurePage, 20);
		} finally {
			loading = false;
		}
	}

	async function handleToggle() {
		if (!stats || toggling) return;
		toggling = true;
		try {
			const result = await toggleMatcher(!stats.matcher_paused);
			if (stats) {
				stats = { ...stats, matcher_paused: result.paused };
			}
		} finally {
			toggling = false;
		}
	}

	async function handleTrigger() {
		if (triggering) return;
		triggering = true;
		try {
			await triggerMatcher();
			await loadRecent();
			await loadStats();
		} finally {
			triggering = false;
		}
	}

	async function handleReset() {
		if (resetting) return;
		resetting = true;
		try {
			await rematch();
			await loadFailures();
			await loadStats();
		} finally {
			resetting = false;
		}
	}

	function switchTab(t: 'recent' | 'failures') {
		tab = t;
		if (t === 'recent') loadRecent();
		else loadFailures();
	}

	function onRecentPageChange(p: number) {
		recentPage = p;
		loadRecent();
	}

	function onFailurePageChange(p: number) {
		failurePage = p;
		loadFailures();
	}

	let matcherPaused = $derived(stats?.matcher_paused ?? false);

	onMount(() => {
		loadStats();
		loadRecent();
		const interval = setInterval(loadStats, 10000);
		return () => clearInterval(interval);
	});
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-xl font-semibold text-text-primary">Matcher</h1>
			<p class="mt-1 text-sm text-text-secondary">Metadata matching and external ID enrichment</p>
		</div>
		<div class="flex items-center gap-3">
			<span
				class="inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium {matcherPaused
					? 'bg-accent-amber/20 text-accent-amber'
					: 'bg-accent-green/20 text-accent-green'}"
			>
				{matcherPaused ? 'Paused' : 'Running'}
			</span>
			<button
				onclick={handleToggle}
				disabled={toggling}
				class="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-tertiary disabled:opacity-50"
			>
				{#if toggling}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{:else if matcherPaused}
					<Play class="h-3.5 w-3.5" />
				{:else}
					<Pause class="h-3.5 w-3.5" />
				{/if}
				{matcherPaused ? 'Resume' : 'Pause'}
			</button>
			<button
				onclick={handleTrigger}
				disabled={triggering}
				class="inline-flex items-center gap-1.5 rounded-md bg-accent-blue px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-accent-blue/80 disabled:opacity-50"
			>
				{#if triggering}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{:else}
					<Zap class="h-3.5 w-3.5" />
				{/if}
				Run Now
			</button>
		</div>
	</div>

	<!-- Stats Row -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		<div class="rounded-lg border border-border bg-bg-secondary p-4">
			<div class="flex items-center justify-between">
				<span class="text-xs font-medium uppercase tracking-wider text-text-secondary">Match Rate</span>
				<Target class="h-4 w-4 text-accent-purple" />
			</div>
			<div class="mt-2 text-2xl font-semibold tracking-tight text-text-primary">
				{$metrics ? formatRate($metrics.match_rate) : '--'}
			</div>
			<div class="mt-2">
				<RateGraph data={$matchRateHistory} color="#8a6baf" height={32} />
			</div>
		</div>
		<StatCard
			label="Success / Attempts"
			value={$metrics
				? `${formatNumber($metrics.match_successes)} / ${formatNumber($metrics.match_attempts)}`
				: '--'}
			subtitle={$metrics && $metrics.match_attempts > 0
				? `${(($metrics.match_successes / $metrics.match_attempts) * 100).toFixed(1)}% success rate`
				: undefined}
			icon={CheckCircle}
			color="text-accent-green"
		/>
		<StatCard
			label="Queue"
			value={stats ? formatNumber(stats.unmatched) : '--'}
			subtitle="Unmatched torrents"
			icon={Clock}
			color="text-accent-amber"
		/>
		<StatCard
			label="Failed"
			value={stats ? formatNumber(stats.failed) : '--'}
			subtitle="Will retry with backoff"
			icon={AlertTriangle}
			color="text-accent-red"
		/>
	</div>

	<!-- Tab Switcher + Content -->
	<div class="rounded-lg border border-border bg-bg-secondary">
		<div class="flex items-center justify-between border-b border-border px-4">
			<div class="flex">
				<button
					onclick={() => switchTab('recent')}
					class="border-b-2 px-4 py-3 text-sm font-medium transition-colors {tab === 'recent'
						? 'border-accent-blue text-text-primary'
						: 'border-transparent text-text-secondary hover:text-text-primary'}"
				>
					Recently Matched
				</button>
				<button
					onclick={() => switchTab('failures')}
					class="border-b-2 px-4 py-3 text-sm font-medium transition-colors {tab === 'failures'
						? 'border-accent-red text-text-primary'
						: 'border-transparent text-text-secondary hover:text-text-primary'}"
				>
					Failures
				</button>
			</div>

			{#if tab === 'failures'}
				<button
					onclick={handleReset}
					disabled={resetting}
					class="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-tertiary disabled:opacity-50"
				>
					{#if resetting}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{:else}
						<RotateCcw class="h-3.5 w-3.5" />
					{/if}
					Reset All Failures
				</button>
			{/if}
		</div>

		{#if loading}
			<div class="flex items-center justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin text-text-secondary" />
			</div>
		{:else if tab === 'recent' && recentData}
			<MatcherList entries={recentData.results ?? []} mode="matched" />
			<Pagination
				page={recentPage}
				total={recentData.total}
				limit={recentData.limit}
				onPageChange={onRecentPageChange}
			/>
		{:else if tab === 'failures' && failureData}
			<MatcherList entries={failureData.results ?? []} mode="failed" />
			<Pagination
				page={failurePage}
				total={failureData.total}
				limit={failureData.limit}
				onPageChange={onFailurePageChange}
			/>
		{/if}
	</div>
</div>
