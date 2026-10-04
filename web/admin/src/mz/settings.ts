/** How the Settings page shows each ~set key. A key missing here still shows, under "Other", as text. */

export type Kind = "bool" | "number" | "duration" | "text" | "long" | "choice";

export interface Meta { group: string; kind: Kind; help: string; choices?: string[]; warn?: string }

export const GROUPS = ["Behaviour", "Screening", "Limits", "Timing", "Model", "Sampling", "Watchers", "Other"];

const sampling = (help: string): Meta => ({ group: "Sampling", kind: "text", help: `${help} "default" leaves it to the server.` });

export const META: Record<string, Meta> = {
  addressed: { group: "Behaviour", kind: "bool", help: "Only answer when addressed by name." },
  trigger: { group: "Behaviour", kind: "text", help: "The word that addresses her; empty uses her nick." },
  responseprefix: { group: "Behaviour", kind: "text", help: "Put before each line she sends." },
  commandprefix: { group: "Behaviour", kind: "text", help: "The character commands start with." },
  ignoreprivate: { group: "Behaviour", kind: "bool", help: "Ignore private messages entirely (no replies, no commands)." },
  showthinkingaction: { group: "Behaviour", kind: "bool", help: "Show a /me while she thinks." },
  showtoolactions: { group: "Behaviour", kind: "bool", help: "Show a /me naming each tool she calls." },
  prompt: { group: "Behaviour", kind: "long", help: "Her system prompt. Edit it in config.yml." },

  screenall: { group: "Screening", kind: "bool", help: "Screen everyone but admins, in and out.",
    warn: "Turning this off stops screening everyone not on the screened list." },

  maxconcurrent: { group: "Limits", kind: "number", help: "Model requests at once, across networks (1-8)." },
  maxreplylines: { group: "Limits", kind: "number", help: "Lines per reply (0 = no limit, up to 20)." },
  botreplylimit: { group: "Limits", kind: "number", help: "Replies to other bots in a row (0-20)." },
  maxtokens: { group: "Limits", kind: "number", help: "Most tokens one reply may produce." },
  maxcontext: { group: "Limits", kind: "number", help: "History size before older turns fold into the recap." },
  chunkmax: { group: "Limits", kind: "number", help: "Most characters in one IRC line." },

  botcooldown: { group: "Timing", kind: "duration", help: "Quiet time that resets the bot reply limit (e.g. 10m)." },
  sessionduration: { group: "Timing", kind: "duration", help: "Idle time before the conversation folds into the recap." },
  apitimeout: { group: "Timing", kind: "duration", help: "How long one model call may take." },

  model: { group: "Model", kind: "text", help: "The model asked for (as ~models switches)." },
  temperature: { group: "Model", kind: "number", help: "Sampling temperature." },
  thinkingeffort: { group: "Model", kind: "choice", choices: ["none", "off", "low", "medium", "high"],
    help: "How much she reasons first. Checked against the model server." },
  openaiurl: { group: "Model", kind: "text", help: "Model server address. Edit it in config.yml." },
  ollamaurl: { group: "Model", kind: "text", help: "Ollama address. Edit it in config.yml." },

  top_p: sampling("Nucleus sampling, 0-1."),
  top_k: sampling("Top-k, a whole number."),
  min_p: sampling("Min-p, 0-1."),
  presence_penalty: sampling("Presence penalty."),
  repeat_penalty: sampling("Repeat penalty."),
  dry_multiplier: sampling("DRY multiplier."),
  dry_base: sampling("DRY base."),
  dry_allowed_length: sampling("DRY allowed length, a whole number."),
  dry_penalty_last_n: sampling("DRY window, a whole number."),

  urlwatcher: { group: "Watchers", kind: "bool", help: "Look at links posted in the channel." },
  urlwatchersilent: { group: "Watchers", kind: "bool", help: "Look at links but don't post the result." },
  opwatcher: { group: "Watchers", kind: "bool", help: "Ask the model to react when she is opped or deopped." },
  opwatchertemplate: { group: "Watchers", kind: "text", help: "What the model is told then: %s is opped/deopped, then the nick." },
};

export const meta = (key: string): Meta => META[key] ?? { group: "Other", kind: "text", help: "" };
