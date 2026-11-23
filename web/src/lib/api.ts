/** The shapes GET /api/week answers with; see internal/store/week.go. */

export type Status = 'planned' | 'done' | 'missed';

export interface Day {
	date: string;
	status: Status | null;
	/** Who set the status: a correction in the app or an n8n automation. */
	source: 'app' | 'n8n' | null;
}

export interface HabitWeek {
	key: string;
	name: string;
	position: number;
	plannable: boolean;
	days: Day[];
}

export interface Summary {
	done: number;
	missed: number;
	planned: number;
	adherence: number | null;
}

export type Priority = 'high' | 'mid' | 'low';

export interface Goal {
	id: string;
	name: string;
	type: string | null;
	priority: Priority | null;
	progress: number | null;
	due: string | null;
	url: string;
}

export interface Streak {
	habit: string;
	name: string;
	count: number;
	unit: 'sessions' | 'days';
}

export interface HabitInsight {
	key: string;
	name: string;
	plannable: boolean;
	streak: number;
	unit: 'sessions' | 'days';
	adherence: number | null;
	prev_adherence: number | null;
}

/**
 * Adherence here covers every habit: a plannable habit's sessions done or missed, any other habit's
 * days since it was first done.
 */
export interface Insights {
	as_of: string;
	streak: Streak | null;
	/** Adherence of the last 8 weeks, oldest first (the last is this week); null = nothing to count. */
	trend: (number | null)[];
	/** Over the last 8 weeks up to as_of, and the 8 weeks before. */
	adherence: number | null;
	prev_adherence: number | null;
	habits: HabitInsight[];
}

export interface Week {
	start: string;
	end: string;
	today: string;
	habits: HabitWeek[];
	summary: Summary;
	prev_adherence: number | null;
	goals: { short_term: Goal[]; perennial: Goal[] };
	insights: Insights;
	updated_at: number | null;
}

/** Why a request didn't go through: no connection, or the Cloudflare Access session expired. */
type Failure = { kind: 'offline' } | { kind: 'expired' };

/**
 * fetch for the API. redirect is manual: an expired Access session answers with a redirect to
 * the login page, which fetch can't follow cross-origin.
 */
async function request(url: string, init?: RequestInit): Promise<Response | Failure> {
	let res: Response;
	try {
		res = await fetch(url, { ...init, redirect: 'manual', credentials: 'same-origin' });
	} catch {
		return { kind: 'offline' };
	}
	// A 403 from Loom itself is a refused cross-origin write, not an expired session.
	const accessRefused = res.status === 403 && !init?.method;
	if (res.type === 'opaqueredirect' || res.status === 401 || accessRefused) return { kind: 'expired' };
	return res;
}

export type WeekResult = { kind: 'ok'; week: Week } | Failure | { kind: 'error'; status: number };

/** Load a week (its Monday, or null for the current one). */
export async function fetchWeek(start: string | null): Promise<WeekResult> {
	const res = await request('/api/week' + (start ? `?start=${start}` : ''));
	if (!(res instanceof Response)) return res;
	if (!res.ok) return { kind: 'error', status: res.status };
	return { kind: 'ok', week: await res.json() };
}

export type SaveResult = { kind: 'ok' } | Failure | { kind: 'error'; message: string };

/** Set one habit's status for one day; null clears it. */
export async function putDay(habit: string, date: string, status: Status | null): Promise<SaveResult> {
	const res = await request(`/api/days/${date}/${habit}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ status })
	});
	if (!(res instanceof Response)) return res;
	if (!res.ok) {
		const body = await res.json().catch(() => null);
		return { kind: 'error', message: body?.error ?? `HTTP ${res.status}` };
	}
	return { kind: 'ok' };
}
