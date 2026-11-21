/** One short message at a time at the bottom of the screen, with an optional action (e.g. Undo). */
export interface Toast {
	message: string;
	action?: { label: string; run: () => unknown };
}

const DURATION_MS = 5000;

export const toast = $state<{ current: Toast | null }>({ current: null });

let timer: ReturnType<typeof setTimeout> | undefined;

export function showToast(t: Toast) {
	clearTimeout(timer);
	toast.current = t;
	timer = setTimeout(dismissToast, DURATION_MS);
}

export function dismissToast() {
	clearTimeout(timer);
	toast.current = null;
}

/** Run the current toast's action, dismissing the toast first. */
export function runToastAction() {
	const action = toast.current?.action;
	dismissToast();
	void action?.run();
}
