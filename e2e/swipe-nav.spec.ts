import { test, expect, type Page } from "@playwright/test";

/** Drag a finger from the middle of the screen by (dx, dy) px in small steps and let go. */
async function swipe(page: Page, dx: number, dy = 0, startX = 195) {
  await page.evaluate(
    ({ dx, dy, startX }) => {
      // Plain events with a `touches` list: desktop WebKit has no Touch constructor.
      const fire = (type: string, x: number, y: number) => {
        const e = new Event(type, { bubbles: true, cancelable: true });
        const touch = { identifier: 1, clientX: x, clientY: y };
        Object.defineProperty(e, "touches", {
          value: type === "touchend" ? [] : [touch],
        });
        document.body.dispatchEvent(e);
      };
      const steps = 10;
      fire("touchstart", startX, 400);
      for (let i = 1; i <= steps; i++)
        fire("touchmove", startX + (dx * i) / steps, 400 + (dy * i) / steps);
      fire("touchend", startX + dx, 400 + dy);
    },
    { dx, dy, startX },
  );
}

const label = (page: Page) => page.locator(".week-label");
// Disabled nav buttons are hidden, so they drop out of role queries.
const next = (page: Page) => page.locator("#next");

async function open(page: Page) {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).not.toHaveText(
    "Loading…",
  );
}

test("swiping right goes back a week and swiping left comes forward again", async ({
  page,
}) => {
  await open(page);
  const thisWeek = await label(page).textContent();

  await swipe(page, 150);
  await expect(next(page)).toBeVisible();
  await expect(label(page)).not.toHaveText(thisWeek!);
  // Back at rest once the new week has slid in.
  await expect(page.getByTestId("swipe-nav")).toHaveCSS("opacity", "1");
  await expect(page.getByTestId("swipe-nav")).toHaveCSS("transform", "none");

  await swipe(page, -150);
  await expect(next(page)).toBeHidden();
  await expect(label(page)).toHaveText(thisWeek!);
});

test("swiping left on this week, a short swipe, a vertical drag or an edge swipe stays put", async ({
  page,
}) => {
  await open(page);
  let fetched = false;
  page.on("request", (r) => r.url().includes("/api/week") && (fetched = true));

  await swipe(page, -150); // no week after this one
  await swipe(page, 40); // too short
  await swipe(page, 30, -200); // mostly up (down would be a pull to refresh)
  await swipe(page, 150, 0, 10); // from the screen's edge

  await expect(page.getByTestId("swipe-nav")).toHaveCSS("transform", "none");
  expect(fetched).toBe(false);
  await expect(next(page)).toBeHidden();
});
