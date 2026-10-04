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
