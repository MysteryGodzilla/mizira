import type { Dashboard } from "../lib/dashboard.svelte";
import { MzApi } from "./api";
import type { Feature, RunState } from "./types";

const EVERY_MS = 5000;
const SHOW_INACTIVE = "mzShowInactive";

function storedFlag(key: string): boolean {
  try { return localStorage.getItem(key) === "1"; } catch { return false; }
}

/** The console's own state, polled beside upstream's Dashboard; changes report through its message line. */
export class MzConsole {
  features = $state<Feature[] | null>(null);
  runState = $state<RunState | null>(null);
  showInactive = $state(storedFlag(SHOW_INACTIVE));
  readonly api: MzApi;
  private timer: ReturnType<typeof setInterval> | undefined;

  constructor(private readonly board: Dashboard, token: string) {
    this.api = new MzApi(token);
  }

  /** A feature's state, or null when unknown (the list didn't load). */
  feature(id: string): Feature | null {
    return this.features?.find((f) => f.id === id) ?? null;
  }

  /** Whether a feature's card is shown: on, or "show inactive". Unknown counts as on, so a failed
   *  load never hides a card that is in use. */
  shows(id: string): boolean {
    return this.showInactive || (this.feature(id)?.active ?? true);
  }

  setShowInactive(on: boolean) {
    this.showInactive = on;
    try { localStorage.setItem(SHOW_INACTIVE, on ? "1" : "0"); } catch { /* lasts for this tab */ }
  }

  async refresh() {
    try {
      const [features, runState] = await Promise.all([this.api.features(), this.api.runState()]);
      Object.assign(this, { features, runState });
    } catch { /* upstream's refresh reports errors; don't say it twice */ }
  }

  /** ~pause, ~stop or ~resume; the card shows the new state straight away. */
  setRunState(state: RunState, done: string) {
    return this.act(async () => { this.runState = (await this.api.setRunState(state)).state; }, done);
  }

  async act(change: () => Promise<unknown>, done: string) {
    await this.board.act(change, done);
    await this.refresh();
  }

  start(): () => void {
    const tick = () => { if (document.visibilityState === "visible") void this.refresh(); };
    void this.refresh();
    this.timer = setInterval(tick, EVERY_MS);
    document.addEventListener("visibilitychange", tick);
    return () => {
      clearInterval(this.timer);
      document.removeEventListener("visibilitychange", tick);
    };
  }
}
