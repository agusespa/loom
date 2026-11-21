/** The shapes GET /api/week answers with; see internal/store/week.go. */

export type Status = 'planned' | 'done' | 'missed';

export interface Day {
	date: string;
	status: Status | null;
	value: number | null;
	note: string | null;
}

export interface HabitWeek {
	key: string;
	name: string;
	position: number;
	plannable: boolean;
	days: Day[];
	done: number;
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

export interface Week {
	start: string;
	end: string;
	today: string;
	habits: HabitWeek[];
	summary: Summary;
	prev_adherence: number | null;
	goals: { short_term: Goal[]; perennial: Goal[] };
	updated_at: number | null;
}

export type WeekResult =
	{ kind: 'ok'; week: Week } | { kind: 'offline' } | { kind: 'expired' } | { kind: 'error'; status: number };

/** Load a week (its Monday, or null for the current one). */
export async function fetchWeek(start: string | null): Promise<WeekResult> {
	const url = '/api/week' + (start ? `?start=${start}` : '');
	let res: Response;
	try {
		// manual: an expired Cloudflare Access session answers with a redirect
		// to the login page, which fetch can't follow cross-origin.
		res = await fetch(url, { redirect: 'manual', credentials: 'same-origin' });
	} catch {
		return { kind: 'offline' };
	}
	if (res.type === 'opaqueredirect' || res.status === 401 || res.status === 403) return { kind: 'expired' };
	if (!res.ok) return { kind: 'error', status: res.status };
	return { kind: 'ok', week: await res.json() };
}
