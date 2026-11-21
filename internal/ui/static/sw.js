"use strict";

// Network first, cache as the fallback: the app is tiny and served from home,
// so it is always fresh when online and still opens offline.
// Bump CACHE when the shell list changes.
const CACHE = "loom-v1";
const SHELL = ["/", "/app.css", "/app.js", "/manifest.webmanifest", "/icon.svg", "/icon-192.png"];

self.addEventListener("install", (event) => {
	event.waitUntil(
		caches
			.open(CACHE)
			.then((c) => c.addAll(SHELL))
			.catch(() => {}) // an expired Access session must not block installing
			.then(() => self.skipWaiting()),
	);
});

self.addEventListener("activate", (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
			.then(() => self.clients.claim()),
	);
});

self.addEventListener("fetch", (event) => {
	const req = event.request;
	const url = new URL(req.url);
	if (req.method !== "GET" || url.origin !== location.origin) return;
	// Only the week endpoint is worth keeping offline.
	if (url.pathname.startsWith("/api/") && url.pathname !== "/api/week") return;
	event.respondWith(networkFirst(req));
});

async function networkFirst(req) {
	const cache = await caches.open(CACHE);
	try {
		const res = await fetch(req);
		// Keep only real answers: not Access redirects or errors.
		if (res.ok && res.type === "basic" && !res.redirected) {
			cache.put(req, res.clone());
		}
		return res;
	} catch (err) {
		const hit = await cache.match(req, { ignoreSearch: req.mode === "navigate" });
		if (!hit) throw err;
		if (!new URL(req.url).pathname.startsWith("/api/")) return hit;
		// Tell the page it is looking at a saved copy.
		const headers = new Headers(hit.headers);
		headers.set("X-Loom-Cache", "hit");
		return new Response(await hit.blob(), { status: hit.status, headers });
	}
}
