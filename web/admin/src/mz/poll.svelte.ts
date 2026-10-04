const EVERY_MS = 5000;

/** Loads a page's data now and every few seconds while the tab is visible; stop with the returned
 *  function. A failed load keeps the last data (the dashboard's message line reports errors). */
export class Poll<T> {
  data = $state<T | null>(null);
  error = $state("");

  constructor(private readonly load: () => Promise<T>) {}

  async refresh() {
    try {
      this.data = await this.load();
      this.error = "";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  start(): () => void {
    const tick = () => { if (document.visibilityState === "visible") void this.refresh(); };
    void this.refresh();
    const timer = setInterval(tick, EVERY_MS);
    document.addEventListener("visibilitychange", tick);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", tick);
    };
  }
}
