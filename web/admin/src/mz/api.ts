import { Api } from "../lib/api";
import type { Bots, Cheatsheet, ExportResult, Conversation, Earlier, Feature, Fold, Look, Memory, People, Profile, ProfileHint, Reset, RunState, Safety, SelfNote, Service, Setting, SettingChange, SortItem, Subject, Tool, ToolSetting } from "./types";

const q = (params: Record<string, string>) => new URLSearchParams(params).toString();

/** Upstream's API plus the console's own endpoints (internal/admin/mz_*.go). */
export class MzApi extends Api {
  models = () => this.call<{ current: string; models: { id: string; name?: string }[] }>("GET", "models");
  switchModel = (model: string) => this.call<{ model: string }>("PUT", "models", { model });
  commands = () => this.call<Cheatsheet>("GET", "commands");
  features = () => this.call<{ features: Feature[] }>("GET", "features").then((r) => r.features);
  services = () => this.call<{ services: Service[] }>("GET", "services").then((r) => r.services);
  runState = () => this.call<{ state: RunState; offline?: string[] }>("GET", "mizira/state");
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

  toolSettings = () => this.call<{ settings: ToolSetting[] }>("GET", "toolsettings").then((r) => r.settings);
  setToolSetting = (key: string, value: string) =>
    this.call<SettingChange>("PUT", `toolsettings/${encodeURIComponent(key)}`, { value });
  resetToolSetting = (key: string) => this.call<SettingChange>("DELETE", `toolsettings/${encodeURIComponent(key)}`);
  tools = () => this.call<{ tools: Tool[] }>("GET", "tools").then((r) => r.tools);
  restrictTool = (spec: string, adminOnly: boolean) =>
    this.call<{ spec: string; adminOnly: boolean }>("PUT", "tools/restrict", { spec, adminOnly });
  switchTool = (spec: string, on: boolean) => this.call<{ spec: string; on: boolean }>("PUT", "tools", { spec, on });
  resetTools = () => this.call<{ reset: boolean }>("DELETE", "tools/switches");

  memorySubjects = (network: string) =>
    this.call<{ subjects: Subject[]; perSubject: number; unsorted: number }>("GET", `memories/subjects?${q({ network })}`);
  memories = (network: string, by: { subject?: string; q?: string }) =>
    this.call<{ memories: Memory[] }>("GET", `memories?${q({ network, subject: by.subject ?? "", q: by.q ?? "" })}`)
      .then((r) => r.memories);
  remember = (network: string, subject: string, fact: string, kind = "", also: string[] = []) =>
    this.call<{ id: number; merged: boolean }>("POST", "memories", { network, subject, fact, kind, also });
  memoryMeta = (network: string, id: number, kind: string, also: string[]) =>
    this.call<{ id: number }>("PUT", `memories/${id}/meta?${q({ network })}`, { kind, also });
  memoryHistory = (network: string, subject: string) =>
    this.call<{ earlier: Earlier[] }>("GET", `memories/history?${q({ network, subject })}`).then((r) => r.earlier);
  sortPreview = (network: string) =>
    this.call<{ suggestions: SortItem[]; unsorted: number }>("POST", "memories/sort/preview", { network });
  sortApply = (network: string, items: SortItem[]) =>
    this.call<{ applied: number; problems: string[] }>("POST", "memories/sort/apply", { network, items });
  profileHints = (network: string, subject: string) =>
    this.call<{ hints: ProfileHint[] }>("GET", `profiles/hints?${q({ network, subject })}`).then((r) => r.hints);
  profiles = (network: string) =>
    this.call<{ profiles: Profile[] }>("GET", `profiles?${q({ network })}`).then((r) => r.profiles);
  setProfile = (network: string, p: Profile) => this.call<{ subject: string }>("PUT", "profiles", { network, ...p });
  setLook = (network: string, subject: string, look: Look) =>
    this.call<{ subject: string }>("PUT", "profiles/look", { network, subject, look });
  deleteProfile = (network: string, subject: string) =>
    this.call<{ subject: string }>("DELETE", `profiles?${q({ network, subject })}`);
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

  selfNotes = (network: string, status: "" | "decided" | SelfNote["status"], query = "", offset = 0, limit = 50) =>
    this.call<{ notes: SelfNote[]; more: boolean }>("GET",
      `selfnotes?${q({ network, status, q: query, offset: String(offset), limit: String(limit) })}`);
  approveSelfNote = (network: string, id: number, text: string) =>
    this.call<{ id: number; memoryId: number; merged: boolean }>("POST", `selfnotes/${id}/approve?${q({ network })}`, { text });
  denySelfNote = (network: string, id: number) => this.call<{ id: number }>("POST", `selfnotes/${id}/deny?${q({ network })}`, {});
}
