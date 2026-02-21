const BASE = '';

export interface MetricsSnapshot {
	dht_nodes_visited: number;
	dht_info_hashes_recv: number;
	torrents_discovered: number;
	metadata_fetched: number;
	metadata_failed: number;
	match_attempts: number;
	match_successes: number;
	match_failures: number;
	torrents_saved: number;
	discovery_rate: number;
	metadata_rate: number;
	match_rate: number;
	uptime_seconds: number;
}

export interface Stats {
	total_torrents: number;
	unmatched: number;
	matched: number;
	failed: number;
	db_size: number;
	last_crawl: number;
	uptime: number;
	start_time: string;
	metrics?: MetricsSnapshot;
}

export interface TorrentResult {
	info_hash: string;
	name: string;
	size: number;
	category: string;
	quality: string;
	imdb_id?: string;
	tmdb_id?: number;
	seeders: number;
	leechers: number;
	discovered_at: number;
}

export interface SearchResponse {
	results: TorrentResult[];
	total: number;
	page: number;
	limit: number;
}

export interface SearchParams {
	q: string;
	page?: number;
	limit?: number;
	category?: number;
	quality?: number;
}

export interface Settings {
	database: {
		backend: string;
		path?: string;
	};
	crawler: {
		enabled: boolean;
		workers: number;
		port: number;
	};
	matcher: {
		enabled: boolean;
		batch_size: number;
		interval: string;
		tmdb_enabled: boolean;
		tvdb_enabled: boolean;
	};
	api: {
		port: number;
		auth_enabled: boolean;
	};
}

async function fetchJSON<T>(url: string): Promise<T> {
	const resp = await fetch(`${BASE}${url}`);
	if (!resp.ok) {
		throw new Error(`HTTP ${resp.status}: ${resp.statusText}`);
	}
	return resp.json();
}

export async function fetchStats(): Promise<Stats> {
	return fetchJSON('/api/stats');
}

export async function search(params: SearchParams): Promise<SearchResponse> {
	const query = new URLSearchParams();
	query.set('q', params.q);
	if (params.page) query.set('page', String(params.page));
	if (params.limit) query.set('limit', String(params.limit));
	if (params.category !== undefined) query.set('category', String(params.category));
	if (params.quality !== undefined) query.set('quality', String(params.quality));
	return fetchJSON(`/api/search?${query}`);
}

export async function fetchSettings(): Promise<Settings> {
	return fetchJSON('/api/settings');
}

export function connectSSE(onMessage: (snap: MetricsSnapshot) => void): EventSource {
	const es = new EventSource(`${BASE}/api/events`);
	es.onmessage = (e) => {
		try {
			onMessage(JSON.parse(e.data));
		} catch {
			// ignore parse errors
		}
	};
	return es;
}
