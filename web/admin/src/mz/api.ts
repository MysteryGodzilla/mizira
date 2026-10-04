import { Api } from "../lib/api";
import type { Conversation, Feature, People, Reset, RunState } from "./types";

const q = (params: Record<string, string>) => new URLSearchParams(params).toString();

/** Upstream's API plus the console's own endpoints (internal/admin/mz_*.go). */
export class MzApi extends Api {
  features = () => this.call<{ features: Feature[] }>("GET", "features").then((r) => r.features);
  runState = () => this.call<{ state: RunState }>("GET", "mizira/state").then((r) => r.state);
  setRunState = (state: RunState) =>
    this.call<{ state: RunState; was: RunState; cancelled: number }>("PUT", "mizira/state", { state });

  conversations = () => this.call<{ conversations: Conversation[] }>("GET", "conversation").then((r) => r.conversations);
  reset = (network: string) => this.call<Reset>("POST", "conversation/reset", { network });
  clearRecap = (network: string) => this.call<{ cleared: boolean }>("DELETE", `recap?${q({ network })}`);

  people = () => this.call<People>("GET", "people");
  ignore = (network: string, nick: string, minutes: number, reason: string) =>
    this.call<{ nick: string; until: number }>("POST", "ignores", { network, nick, minutes, reason });
  unignore = (network: string, nick: string) => this.call<{ nick: string }>("DELETE", `ignores?${q({ network, nick })}`);
  screen = (network: string, nick: string) => this.call<{ nick: string; dropped: number }>("POST", "screens", { network, nick });
  unscreen = (nick: string) => this.call<{ nick: string }>("DELETE", `screens?${q({ nick })}`);
  clearSuspicion = (network: string, key: string) =>
    this.call<{ key: string }>("DELETE", `suspicion?${q({ network, key })}`);
}
