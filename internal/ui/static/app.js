"use strict";

// Read-only dashboard: everything comes from GET /api/week, which n8n keeps
// up to date. The service worker keeps the last answer for offline opens.

const $ = (id) => document.getElementById(id);
const DAY_LETTERS = ["M", "T", "W", "T", "F", "S", "S"];
const STATUS_LABEL = { done: "Done", missed: "Missed", planned: "Planned" };
const STATUS_GLYPH = { done: "✓", missed: "×", planned: "" };

let current = null; // the Monday shown, null = this week
let latestStart = null; // this week's Monday, from the server
let selected = null;

function el(tag, attrs = {}, ...children) {
	const node = document.createElement(tag);
	for (const [k, v] of Object.entries(attrs)) {
		if (v === undefined || v === null || v === false) continue;
		if (k === "class") node.className = v;
		else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
		else node.setAttribute(k, v === true ? "" : v);
	}
	for (const c of children.flat()) {
		if (c === null || c === undefined || c === false) continue;
		node.append(c instanceof Node ? c : document.createTextNode(String(c)));
	}
	return node;
}

function parseDate(s) {
	const [y, m, d] = s.split("-").map(Number);
	return new Date(Date.UTC(y, m - 1, d));
}

function fmt(s, opts) {
	return parseDate(s).toLocaleDateString("en-GB", { timeZone: "UTC", ...opts });
}

function addDays(s, n) {
	const d = parseDate(s);
	d.setUTCDate(d.getUTCDate() + n);
	return d.toISOString().slice(0, 10);
}

function isoWeek(s) {
	// The ISO week is the one holding this week's Thursday.
	const thu = parseDate(s);
	thu.setUTCDate(thu.getUTCDate() + 3 - ((thu.getUTCDay() + 6) % 7));
	const jan1 = Date.UTC(thu.getUTCFullYear(), 0, 1);
	return 1 + Math.floor((thu - jan1) / 864e5 / 7);
}

function pct(x) {
	return x === null || x === undefined ? "–" : Math.round(x * 100) + "%";
}

function ago(ms) {
	const min = Math.round((Date.now() - ms) / 60000);
	if (min < 1) return "just now";
	if (min < 60) return `${min} min ago`;
	const h = Math.round(min / 60);
	if (h < 48) return `${h} h ago`;
	return `${Math.round(h / 24)} days ago`;
}

// ---------- loading ----------

async function load() {
	const url = "/api/week" + (current ? `?start=${current}` : "");
	let res;
	try {
		// manual: an expired Cloudflare Access session answers with a redirect
		// to the login page, which fetch can't follow cross-origin.
		res = await fetch(url, { redirect: "manual", credentials: "same-origin" });
	} catch {
		return banner("You're offline and there's no saved copy of this week yet.");
	}
	if (res.type === "opaqueredirect" || res.status === 401 || res.status === 403) {
		return banner("Your session expired. ", el("a", { href: "/", onclick: () => location.reload() }, "Sign in again"));
	}
	if (!res.ok) return banner(`Couldn't load this week (HTTP ${res.status}).`);

	const week = await res.json();
	const offline = res.headers.get("X-Loom-Cache") === "hit";
	banner(offline ? "Offline: showing the last saved copy." : null);
	render(week);
}

function banner(...content) {
	const b = $("banner");
	b.replaceChildren(...content.filter(Boolean));
	b.hidden = !content.some(Boolean);
	if ($("headline").textContent === "Loading…") $("headline").textContent = "Loom";
}

// ---------- rendering ----------

function render(w) {
	if (!current) latestStart = w.start;
	const isCurrent = w.start === latestStart || (!latestStart && !current);
	$("next").disabled = isCurrent;

	$("range").textContent = `Week ${isoWeek(w.start)} · ${fmt(w.start, { day: "numeric", month: "short" })}–${fmt(w.end, { day: "numeric", month: "short" })}`;

	const a = w.summary.adherence;
	const adh = $("adherence");
	adh.textContent = pct(a);
	adh.className = "big" + (a === null ? "" : a >= 0.7 ? " good" : a < 0.5 ? " bad" : "");
	$("vs").textContent = w.prev_adherence === null ? "" : `vs ${pct(w.prev_adherence)} last wk`;
	$("headline").textContent = headline(w, isCurrent);

	renderCards(w);
	renderGrid(w, isCurrent);
	renderGoals($("goals-short"), w.goals.short_term, w.today);
	renderGoals($("goals-perennial"), w.goals.perennial, w.today);
	$("synced").textContent = w.updated_at ? `Updated ${ago(w.updated_at)}` : "No data from n8n yet";
}

function headline(w, isCurrent) {
	const { done, missed, planned, adherence } = w.summary;
	if (done + missed + planned === 0) return isCurrent ? "Nothing planned yet" : "Quiet week";
	if (adherence === null) return planned ? `${planned} planned` : "Week ahead";
	if (adherence >= 0.85) return isCurrent ? "Great week so far" : "Great week";
	if (adherence >= 0.6) return isCurrent ? "Good week so far" : "Good week";
	return isCurrent ? "Room to catch up" : "Tough week";
}

function renderCards(w) {
	$("cards").replaceChildren(
		...w.habits.map((h) => {
			let value;
			if (h.plannable) {
				const scheduled = h.days.filter((d) => d.status).length;
				value = [h.done, el("span", {}, `/${scheduled}`)];
			} else {
				value = [h.done, el("span", {}, h.done === 1 ? " day" : " days")];
			}
			return el("div", { class: "card" }, el("div", { class: "muted small" }, h.name), el("div", { class: "v" }, ...value));
		}),
	);
}

function renderGrid(w, isCurrent) {
	const dates = w.habits[0]?.days.map((d) => d.date) ?? [];
	const head = el(
		"div",
		{ class: "row head", role: "row" },
		el("span"),
		...dates.map((date, i) => el("span", { class: date === w.today ? "today-l" : null, role: "columnheader" }, DAY_LETTERS[i])),
	);
	const rows = w.habits.map((h) =>
		el(
			"div",
			{ class: "row", role: "row" },
			el("span", { class: "name", role: "rowheader" }, h.name),
			...h.days.map((d) => {
				const cls = ["d", d.status ?? "empty"];
				if (d.date > w.today) cls.push("future");
				if (isCurrent && d.date === w.today) cls.push("today");
				const key = `${h.key}/${d.date}`;
				if (selected === key) cls.push("sel");
				return el(
					"button",
					{
						class: cls.join(" "),
						role: "cell",
						"aria-label": `${h.name}, ${fmt(d.date, { weekday: "long" })}: ${STATUS_LABEL[d.status] ?? "nothing"}`,
						onclick: () => {
							selected = selected === key ? null : key;
							showDetail(h, d);
							renderGrid(w, isCurrent);
						},
					},
					STATUS_GLYPH[d.status] ?? "",
				);
			}),
		),
	);
	$("grid").replaceChildren(head, ...rows);
	if (!selected) $("detail").innerHTML = "&nbsp;";
}

function showDetail(h, d) {
	if (!selected) return;
	const parts = [fmt(d.date, { weekday: "short", day: "numeric", month: "short" }), h.name, STATUS_LABEL[d.status] ?? "Nothing logged"];
	if (d.note) parts.push(d.note);
	if (d.value !== null && d.value !== undefined) parts.push(h.key === "meditation" ? `${Math.round(d.value)} min` : String(d.value));
	$("detail").textContent = parts.join(" · ");
}

function renderGoals(container, goals, today) {
	if (!goals.length) {
		container.replaceChildren(el("div", { class: "empty-list" }, "No open goals."));
		return;
	}
	container.replaceChildren(
		el(
			"div",
			{ class: "grid" },
			...goals.map((g) => {
				const meta = [];
				if (g.progress !== null) meta.push(pct(g.progress));
				let due = null;
				if (g.due) {
					const overdue = g.due < today && (g.progress ?? 0) < 1;
					due = el("span", { class: overdue ? "overdue" : null }, fmt(g.due, { day: "numeric", month: "short", year: g.due.slice(0, 4) === today.slice(0, 4) ? undefined : "numeric" }));
				}
				return el(
					"a",
					{ class: "goal", href: g.url, target: "_blank", rel: "noopener" },
					el(
						"div",
						{ class: "goal-top" },
						el("span", { class: "goal-name" }, g.priority ? el("span", { class: `prio ${g.priority}`, title: `${g.priority} priority` }) : null, g.name),
						el("span", { class: "goal-meta" }, meta.join(" "), meta.length && due ? " · " : "", due),
					),
					el("div", { class: "bar" }, el("span", { style: `width:${Math.round(Math.min(1, g.progress ?? 0) * 100)}%` })),
				);
			}),
		),
	);
}

// ---------- navigation ----------

$("prev").addEventListener("click", () => {
	current = addDays(current ?? latestStart, -7);
	selected = null;
	load();
});
$("next").addEventListener("click", () => {
	const next = addDays(current ?? latestStart, 7);
	current = next >= latestStart ? null : next;
	selected = null;
	load();
});
document.addEventListener("visibilitychange", () => {
	if (document.visibilityState === "visible") load();
});

if ("serviceWorker" in navigator) {
	navigator.serviceWorker.register("/sw.js").catch(() => {});
}

load();
