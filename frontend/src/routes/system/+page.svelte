<script lang="ts">
	import { fetchSystemInfo, type SystemInfo } from '$lib/api';
	import SettingsSection from '$lib/components/SettingsSection.svelte';
	import SettingsRow from '$lib/components/SettingsRow.svelte';
	import { Loader2 } from 'lucide-svelte';
	import { onMount, onDestroy } from 'svelte';

	let system = $state<SystemInfo | null>(null);
	let error = $state<string | null>(null);
	let interval: ReturnType<typeof setInterval> | null = null;

	async function load() {
		try {
			system = await fetchSystemInfo();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load system info';
		}
	}

	onMount(() => {
		load();
		interval = setInterval(load, 30_000);
	});

	onDestroy(() => {
		if (interval) clearInterval(interval);
	});

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(1024));
		return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
	}

	function formatUptime(seconds: number): string {
		const d = Math.floor(seconds / 86400);
		const h = Math.floor((seconds % 86400) / 3600);
		const m = Math.floor((seconds % 3600) / 60);
		if (d > 0) return `${d}d ${h}h ${m}m`;
		if (h > 0) return `${h}h ${m}m`;
		return `${m}m`;
	}

	function formatTimestamp(ts: number): string {
		if (ts === 0) return '--';
		return new Date(ts * 1000).toLocaleString();
	}
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-xl font-semibold text-text-primary">System</h1>
		<p class="mt-1 text-sm text-text-secondary">Database, scheduled tasks, and process info</p>
	</div>

	{#if error}
		<div class="rounded-lg border border-accent-red/30 bg-accent-red/10 p-4 text-sm text-accent-red">
			{error}
		</div>
	{:else if !system}
		<div class="flex items-center justify-center py-12">
			<Loader2 class="h-6 w-6 animate-spin text-text-secondary" />
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
			<SettingsSection title="Database">
				<SettingsRow label="Backend" value={system.database.backend} />
				{#if system.database.path}
					<SettingsRow label="Path" value={system.database.path} />
				{/if}
				<SettingsRow label="Size" value={formatBytes(system.database.size)} />
				<SettingsRow label="Total Torrents" value={system.total_torrents.toLocaleString()} />
				<SettingsRow label="Matched" value={system.matched.toLocaleString()} />
				<SettingsRow label="Unmatched" value={system.unmatched.toLocaleString()} />
				<SettingsRow label="Failed" value={system.failed.toLocaleString()} />
				<SettingsRow label="Rejected Hashes" value={system.rejected_hashes_count.toLocaleString()} />
			</SettingsSection>

			<SettingsSection title="Process">
				<SettingsRow label="Uptime" value={formatUptime(system.process.uptime)} />
				<SettingsRow label="Started" value={new Date(system.process.start_time).toLocaleString()} />
				<SettingsRow label="Go Version" value={system.process.go_version} />
				<SettingsRow label="OS / Arch" value={system.process.os_arch} />
			</SettingsSection>
		</div>

		<div class="rounded-lg border border-border bg-bg-secondary p-5">
			<h3 class="mb-4 text-sm font-medium uppercase tracking-wider text-text-secondary">Scheduled Tasks</h3>
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-border text-left text-text-secondary">
							<th class="pb-2 pr-4 font-medium">Task</th>
							<th class="pb-2 pr-4 font-medium">Interval</th>
							<th class="pb-2 pr-4 font-medium">Last Run</th>
							<th class="pb-2 pr-4 font-medium">Result</th>
							<th class="pb-2 font-medium">Next Run</th>
						</tr>
					</thead>
					<tbody>
						{#each system.tasks as task (task.name)}
							<tr class="border-b border-border/50 last:border-0">
								<td class="py-2.5 pr-4 font-medium text-text-primary">{task.name}</td>
								<td class="py-2.5 pr-4 text-text-secondary">{task.interval}</td>
								<td class="py-2.5 pr-4 text-text-secondary">{formatTimestamp(task.last_run)}</td>
								<td class="py-2.5 pr-4">
									{#if task.last_run === 0}
										<span class="text-text-secondary">--</span>
									{:else if task.last_error}
										<span class="text-accent-red">{task.last_result}</span>
									{:else}
										<span class="text-accent-green">{task.last_result}</span>
									{/if}
								</td>
								<td class="py-2.5 text-text-secondary">{formatTimestamp(task.next_run)}</td>
							</tr>
						{/each}
						{#if system.tasks.length === 0}
							<tr>
								<td colspan="5" class="py-4 text-center text-text-secondary">No tasks registered</td>
							</tr>
						{/if}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</div>
