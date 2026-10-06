import { Api } from "../lib/api";
import type { Bots, Cheatsheet, ExportResult, Conversation, Feature, Fold, Memory, People, Reset, RunState, Safety, SelfNote, Setting, SettingChange, Subject, Tool } from "./types";

const q = (params: Record<string, string>) => new URLSearchParams(params).toString();

/** Upstream's API plus the console's own endpoints (internal/admin/mz_*.go). */
export class MzApi extends Api {
  commands = () => this.call<Cheatsheet>("GET", "commands");
  features = () => this.call<{ features: Feature[] }>("GET", "features").then((r) => r.features);
  runState = () => this.call<{ state: RunState }>("GET", "mizira/state").then((r) => r.state);
  setRunState = (state: RunState) =>
    this.call<{ state: RunState; was: RunState; cancelled: number }>("PUT", "mizira/state", { state });

  conversations = () => this.call<{ conversations: Conversation[] }>("GET", "conversation").then((r) => r.conversations);
  reset = (network: string) => this.call<Reset>("POST", "conversation/reset", { network });
  fold = (network: string) => this.call<Fold>("POST", "conversation/fold", { network });
  clearRecap = (network: string) => this.call<{ cleared: boolean }>("DELETE", `recap?${q({ network })}`);

  people = () => this.call<People>("GET", "people");
  ignore = (network: string, nick: string, minutes: number, reason: string) =>
    this.call<{ nick: string; until: number }>("POST", "ignores", { network, nick, minutes, reason });
  unignore = (network: string, nick: string) => this.call<{ nick: string }>("DELETE", `ignores?${q({ network, nick })}`);
  screen = (network: string, nick: string) => this.call<{ nick: string; dropped: number }>("POST", "screens", { network, nick });
  unscreen = (nick: string) => this.call<{ nick: string }>("DELETE", `screens?${q({ nick })}`);
  clearSuspicion = (network: string, key: string) =>
    this.call<{ key: string }>("DELETE", `suspicion?${q({ network, key })}`);

  bots = () => this.call<Bots>("GET", "bots");
  addBot = (kind: "nick" | "prefix", value: string) => this.call<{ value: string }>("POST", "bots", { kind, value });
  removeBot = (kind: "nick" | "prefix", value: string) =>
    this.call<{ value: string }>("DELETE", `bots?${q({ kind, value })}`);

  settings = () => this.call<{ settings: Setting[] }>("GET", "settings").then((r) => r.settings);
  settingsAndLists = () => this.call<{ settings: Setting[]; lists: string[] }>("GET", "settings");
  exportSettings = () => this.call<ExportResult>("POST", "settings/export");
  resetAll = () => this.call<{ reset: string[]; onRestart: string[] }>("POST", "settings/reset-all");
  set = (key: string, value: string) => this.call<SettingChange>("PUT", `settings/${encodeURIComponent(key)}`, { value });
  resetSetting = (key: string) => this.call<SettingChange>("DELETE", `settings/${encodeURIComponent(key)}`);

  tools = () => this.call<{ tools: Tool[] }>("GET", "tools").then((r) => r.tools);
  switchTool = (spec: string, on: boolean) => this.call<{ spec: string; on: boolean }>("PUT", "tools", { spec, on });
  resetTools = () => this.call<{ reset: boolean }>("DELETE", "tools/switches");

  memorySubjects = (network: string) =>
    this.call<{ subjects: Subject[]; perSubject: number }>("GET", `memories/subjects?${q({ network })}`);
  memories = (network: string, by: { subject?: string; q?: string }) =>
    this.call<{ memories: Memory[] }>("GET", `memories?${q({ network, subject: by.subject ?? "", q: by.q ?? "" })}`)
      .then((r) => r.memories);
  remember = (network: string, subject: string, fact: string) =>
    this.call<{ id: number; merged: boolean }>("POST", "memories", { network, subject, fact });
  editMemory = (network: string, id: number, fact: string) =>
    this.call<{ id: number }>("PUT", `memories/${id}?${q({ network })}`, { fact });
  lockMemory = (network: string, id: number, locked: boolean) =>
    this.call<{ id: number; locked: boolean }>(locked ? "PUT" : "DELETE", `memories/${id}/lock?${q({ network })}`);
  forgetMemory = (network: string, id: number) => this.call<{ id: number }>("DELETE", `memories/${id}?${q({ network })}`);

  compactPreview = (network: string, subject: string) =>
    this.call<{ facts: string[]; basedOn: number[] }>("POST", "memories/compact/preview", { network, subject });
  compactApply = (network: string, subject: string, basedOn: number[], facts: string[]) =>
    this.call<{ stored: number }>("POST", "memories/compact/apply", { network, subject, basedOn, facts });

  safety = (days: number, kind = "", who = "") => this.call<Safety>("GET", `safety?${q({ days: String(days), kind, who })}`);

  selfNotes = (network: string, status: "" | SelfNote["status"]) =>
    this.call<{ notes: SelfNote[] }>("GET", `selfnotes?${q({ network, status })}`).then((r) => r.notes);
  approveSelfNote = (network: string, id: number, text: string) =>
    this.call<{ id: number; memoryId: number; merged: boolean }>("POST", `selfnotes/${id}/approve?${q({ network })}`, { text });
  denySelfNote = (network: string, id: number) => this.call<{ id: number }>("POST", `selfnotes/${id}/deny?${q({ network })}`, {});
}
