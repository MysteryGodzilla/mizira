import type { Dashboard } from "../lib/dashboard.svelte";
import { MzApi } from "./api";
import type { Feature, RunState, SelfNote, Setting, Tool } from "./types";

const EVERY_MS = 5000;
const SHOW_INACTIVE = "mzShowInactive";

function storedFlag(key: string): boolean {
  try { return localStorage.getItem(key) === "1"; } catch { return false; }
}

/** The console's own state, polled beside upstream's Dashboard; changes report through its message line. */
export class MzConsole {
  features = $state<Feature[] | null>(null);
  runState = $state<RunState | null>(null);
  // For the "differs from config.yml" banner, and the Settings and Tools pages.
  settings = $state<Setting[]>([]);
  /** List settings (admins, screening, bots) changed at runtime; they show in config.yml only after an export. */
  lists = $state<string[]>([]);
  tools = $state<Tool[]>([]);
  // Self-notes waiting for a decision, on every network.
  pendingNotes = $state<SelfNote[]>([]);
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

  /** What differs from config.yml right now: +set values and tool switches. */
  get differences(): string[] {
    return [
      ...this.settings.filter((s) => s.overridden).map((s) => `${s.key} ${s.value} (config.yml: ${s.default || "empty"})`),
      ...this.tools.filter((t) => t.switched).map((t) => `${t.spec} ${t.loaded ? "on" : "off"}`),
      ...this.lists.map((l) => `${l} list changed`),
    ];
  }

  setShowInactive(on: boolean) {
    this.showInactive = on;
    try { localStorage.setItem(SHOW_INACTIVE, on ? "1" : "0"); } catch { /* lasts for this tab */ }
  }

  async refresh() {
    try {
      const [features, runState, { settings, lists }, tools] = await Promise.all([
        this.api.features(), this.api.runState(), this.api.settingsAndLists(), this.api.tools(),
      ]);
      const networks = this.board.status?.networks.map((n) => n.name) ?? [];
      const pendingNotes = (await Promise.all(networks.map((n) => this.api.selfNotes(n, "pending")))).flat();
      Object.assign(this, { features, runState, settings, lists, tools, pendingNotes });
    } catch { /* upstream's refresh reports errors; don't say it twice */ }
  }

  /** ~pause, ~stop or ~resume; the card shows the new state straight away. */
  setRunState(state: RunState, done: string) {
    return this.act(async () => { this.runState = (await this.api.setRunState(state)).state; }, done);
  }

  /** Show a note on the dashboard's message line. */
  say(text: string) {
    this.board.message = { text, bad: false };
  }

  /** Run a change through the dashboard's message line; true if it went through. */
  async act(change: () => Promise<unknown>, done: string): Promise<boolean> {
    let ok = false;
    await this.board.act(async () => { await change(); ok = true; }, done);
    await this.refresh();
    return ok;
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
