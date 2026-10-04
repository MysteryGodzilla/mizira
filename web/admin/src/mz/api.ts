import { Api } from "../lib/api";
import type { Feature, RunState } from "./types";

/** Upstream's API plus the console's own endpoints (internal/admin/mz_*.go). */
export class MzApi extends Api {
  features = () => this.call<{ features: Feature[] }>("GET", "features").then((r) => r.features);
  runState = () => this.call<{ state: RunState }>("GET", "mizira/state").then((r) => r.state);
  setRunState = (state: RunState) =>
    this.call<{ state: RunState; was: RunState; cancelled: number }>("PUT", "mizira/state", { state });
}
