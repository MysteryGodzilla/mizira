<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { RunState } from "./types";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  let state = $derived(mz.runState);
  const tone = { running: "good", paused: "busy", stopped: "bad" } as const;
  const about: Record<RunState, string> = {
    running: "Answering as normal.",
    paused: "Paused: nothing new starts; anything already running finishes. Same as ~pause.",
    stopped: "Stopped: everything running was cancelled and nothing new starts. Same as ~stop.",
  };

  function stop() {
    if (confirm("Stop Mizira?\n\nEverything she's doing is cancelled and she won't answer until resumed.")) {
      void mz.setRunState("stopped", "stopped Mizira");
    }
  }
</script>

<Card title="Mizira">
  {#snippet actions()}
    {#if state === "running" && !mz.offline.length}
      <button onclick={() => mz.setRunState("paused", "paused Mizira")}>Pause</button>
    {:else if state && !mz.offline.length}
      <button onclick={() => mz.setRunState("running", "resumed Mizira")}>Resume</button>
    {/if}
    {#if state && state !== "stopped" && !mz.offline.length}<button class="danger" onclick={stop}>Stop</button>{/if}
  {/snippet}
  {#if state}
    {#if mz.offline.length}
      <p><Badge text="off IRC" tone="bad" /><span class="sub">Not on IRC this run: see the note at the top.</span></p>
    {:else}
      <p><Badge text={state} tone={tone[state]} /><span class="sub">{about[state]}</span></p>
    {/if}
    {#if mz.pendingNotes.length}
      <p class="notes"><a href="#memories">{mz.pendingNotes.length} self-note{mz.pendingNotes.length === 1 ? "" : "s"} waiting for you</a></p>
    {/if}
  {:else}
    <p class="empty">Loading…</p>
  {/if}
</Card>

<style>
  p { margin: 0; }
  .notes { margin-top: 8px; font-size: 14px; }
  .notes a { color: var(--accent); }
</style>
