import { Api, Unauthorized } from "./api";
import { session } from "./session.svelte";
import type { Gpu, Radio, Reminder, Status, Task } from "./types";

const EVERY_MS = 5000;

/** Everything the page shows, refreshed every few seconds while the tab is visible. */
export class Dashboard {
  status = $state<Status | null>(null);
  tasks = $state<Task[]>([]);
  reminders = $state<Reminder[]>([]);
  gpu = $state<Gpu | null>(null);
  radio = $state<Radio | null>(null);
  /** poll: a failed refresh, cleared once polling works again; an action's refusal stays until dismissed. */
  message = $state<{ text: string; bad: boolean; poll?: boolean } | null>(null);
  readonly api: Api;
  private timer: ReturnType<typeof setInterval> | undefined;

  constructor(token: string) {
    this.api = new Api(token);
  }

  async refresh() {
    try {
      const [status, tasks, reminders, gpu, radio] = await Promise.all([
        this.api.status(), this.api.tasks(), this.api.reminders(), this.api.gpu(), this.api.radio(),
      ]);
      Object.assign(this, { status, tasks, reminders, gpu, radio });
      if (this.message?.poll) this.message = null;
    } catch (e) {
      this.fail(e, true);
    }
  }

  /** Run a change, say how it went, and show the result. */
  async act(change: () => Promise<unknown>, done: string) {
    try {
      await change();
      this.message = { text: done, bad: false };
      await this.refresh();
    } catch (e) {
      this.fail(e);
    }
  }

  private fail(e: unknown, poll = false) {
    if (e instanceof Unauthorized) session.refused();
    this.message = { text: e instanceof Error ? e.message : String(e), bad: true, poll };
  }

  /** Poll until the returned function is called; a hidden tab doesn't poll. */
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
