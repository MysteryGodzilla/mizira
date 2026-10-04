// Mirrors internal/admin/mz_mizira.go.

export type RunState = "running" | "paused" | "stopped";

/** An optional part of the bot; its cards are hidden while it's off. */
export interface Feature { id: string; label: string; active: boolean; how: string }
