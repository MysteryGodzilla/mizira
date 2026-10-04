<script lang="ts">
  import type { Dashboard } from "../lib/dashboard.svelte";
  import type { Task } from "../lib/types";
  import Card from "./Card.svelte";
  import TaskRow from "./TaskRow.svelte";
  let { board }: { board: Dashboard } = $props();

  const views = {
    active: { label: "Active", show: (t: Task) => t.active && t.kind !== "schedule" },
    schedules: { label: "Schedules", show: (t: Task) => t.active && t.kind === "schedule" },
    finished: { label: "Finished", show: (t: Task) => !t.active },
  } as const;
  type View = keyof typeof views;
  let view = $state<View>("active");
  let shown = $derived(board.tasks.filter(views[view].show));
</script>

<Card title="Tasks, goals and schedules">
  {#snippet actions()}
    {#each board.status?.networks ?? [] as n (n.name)}
      <button onclick={() => board.act(() => board.api.setPaused(n.name, !n.paused), `${n.paused ? "resumed" : "paused"} work on ${n.name}`)}>
        {n.paused ? "Resume" : "Pause"} work on {n.name}
      </button>
    {/each}
  {/snippet}
  {#each (board.status?.networks ?? []).filter((n) => n.paused) as n (n.name)}
    <p class="sub">Work on {n.name} is paused: no tasks, goals or schedules start until it's resumed.</p>
  {/each}
  <div class="tabs" role="tablist">
    {#each Object.entries(views) as [key, v] (key)}
      <button role="tab" aria-selected={view === key} class:on={view === key} onclick={() => (view = key as View)}>
        {v.label} ({board.tasks.filter(v.show).length})
      </button>
    {/each}
  </div>
  {#if shown.length}
    <ul class="rows">
      {#each shown as task (task.network + task.id)}
        <TaskRow {task} {board} />
      {/each}
    </ul>
  {:else}
    <p class="empty">Nothing here.</p>
  {/if}
</Card>

<style>
  .tabs { display: flex; gap: 6px; margin-bottom: 10px; flex-wrap: wrap; }
  .on { border-color: var(--accent); color: var(--accent); }
  p.sub { margin: 0 0 10px; }
</style>
