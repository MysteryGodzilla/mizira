// Mirrors internal/admin/mz_mizira.go and mz_people.go.

export type RunState = "running" | "paused" | "stopped";

/** An optional part of the bot; its cards are hidden while it's off. */
export interface Feature { id: string; label: string; active: boolean; how: string }

export interface Line { role: string; text: string }

export interface Conversation {
  network: string; channel: string; messages: number; tokens: number; maxContext: number; lastUsed: number;
  persona: string; personaBy: string; recap: string; recent: Line[];
}

export interface Reset { personaCleared: boolean; modelRestored: string; cancelled: number }

export interface Ignore { network: string; nick: string; until: number; kind: string; by: string; reason: string }

export interface Score { network: string; key: string; score: number }

export interface People {
  ignores: Ignore[]; screened: { in: string[]; out: string[] }; suspicion: Score[]; quarantineAt: number;
}

// Mirrors internal/admin/mz_settings.go.

export interface Bots { nicks: string[]; prefixes: string[]; replyLimit: string; cooldown: string }

export interface Setting { key: string; value: string; default: string; overridden: boolean; editable: boolean }

export interface SettingChange { key: string; value: string; warning?: string }

export interface Tool {
  spec: string; kind: "native" | "work" | "plugin" | "mcp"; description: string; loaded: boolean; names: string[];
  inConfig: boolean; switched: boolean; adminOnly: boolean;
}

// Mirrors internal/admin/mz_memories.go.

export interface Subject { subject: string; count: number; room: boolean }

export interface Memory { id: number; subject: string; fact: string; author: string; created: number; sameAs?: number }

// Mirrors internal/admin/mz_selfnotes.go.

export interface SelfNote {
  id: number; text: string; why: string; status: "pending" | "approved" | "denied"; created: number;
  decidedBy: string; decidedAt: number; memoryId: number;
}

// Mirrors internal/admin/mz_safety.go.

export interface SafetyEvent { time: number; kind: string; event: string; who: string; channel: string; detail: string; suspicion?: string }

export interface PersonCount { who: string; total: number; byKind: Record<string, number> }

export interface Safety {
  events: SafetyEvent[]; people: PersonCount[]; kinds: Record<string, number>; days: number; truncated: boolean;
}
