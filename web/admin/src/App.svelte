<script lang="ts">
  import { session } from "./lib/session.svelte";
  import { Dashboard } from "./lib/dashboard.svelte";
  import Login from "./components/Login.svelte";
  import Overview from "./components/Overview.svelte";
  import Inflight from "./components/Inflight.svelte";
  import GpuQueue from "./components/GpuQueue.svelte";
  import Radio from "./components/Radio.svelte";
  import Tasks from "./components/Tasks.svelte";
  import Reminders from "./components/Reminders.svelte";
  import Logs from "./components/Logs.svelte";
  import Thinking from "./components/Thinking.svelte";
  import { MzConsole } from "./mz/console.svelte";
  import MiziraCard from "./mz/MiziraCard.svelte";
  import Feature from "./mz/Feature.svelte";

  // Signed in (by the auth proxy, or with a token) means a dashboard; signing out drops it and stops
  // its polling.
  let token = $derived(session.state === "token" ? session.token : "");
  let board = $derived(session.state === "proxy" || session.state === "token" ? new Dashboard(token) : null);
  $effect(() => board?.start());
  // Mizira's console additions, polled alongside upstream's dashboard.
  let mz = $derived(board ? new MzConsole(board, token) : null);
  $effect(() => mz?.start());
  $effect(() => { void session.check(); });

  // The view lives in the URL hash, so a reload or a bookmark keeps it.
  const VIEWS = { dashboard: "Dashboard", logs: "Logs", thinking: "Thinking" } as const;
  type View = keyof typeof VIEWS;
  const fromHash = (): View => { const h = location.hash.slice(1); return h in VIEWS ? (h as View) : "dashboard"; };
  let view = $state<View>(fromHash());
  // A tab for a feature that's off is hidden like its card.
  const tabFeature: Partial<Record<View, string>> = { thinking: "thinking" };
  let tabs = $derived(Object.entries(VIEWS).filter(([id]) => !mz || !tabFeature[id as View] || mz.shows(tabFeature[id as View]!)));
</script>

<svelte:window onhashchange={() => (view = fromHash())} />

<main>
  <header>
    <h1>Bot operator</h1>
    {#if board}
      <nav>
        {#each tabs as [id, label] (id)}
          <a href="#{id}" class:on={view === id} aria-current={view === id ? "page" : undefined}>{label}</a>
        {/each}
      </nav>
      {#if mz}
        <label class="inactive"><input type="checkbox" checked={mz.showInactive}
          onchange={(e) => mz.setShowInactive(e.currentTarget.checked)} /> Show inactive</label>
      {/if}
    {/if}
  </header>
  {#if board}
    {#if board.message}
      <p class="message" class:bad={board.message.bad} role="status">{board.message.text}</p>
    {/if}
    {#if view === "logs"}
      <Logs {token} />
    {:else if view === "thinking"}
      <Feature {mz} id="thinking"><Thinking {token} /></Feature>
    {:else}
      {#if mz}<MiziraCard {mz} />{/if}
      <Overview {board} />
      <Inflight {board} />
      <Feature {mz} id="gpu"><GpuQueue {board} /></Feature>
      <Feature {mz} id="radio"><Radio {board} /></Feature>
      <Feature {mz} id="work"><Tasks {board} /></Feature>
      <Feature {mz} id="reminders"><Reminders {board} /></Feature>
    {/if}
  {:else if session.state === "out"}
    <Login />
  {/if}
</main>

<style>
  main { max-width: 980px; margin: 0 auto; padding: 28px 16px 64px; }
  header { display: flex; align-items: baseline; gap: 18px; margin-bottom: 18px; flex-wrap: wrap; }
  h1 { font-size: 14px; letter-spacing: .12em; text-transform: uppercase; color: var(--muted); margin: 0; }
  nav { display: flex; gap: 14px; }
  nav a { font-size: 14px; color: var(--muted); text-decoration: none; font-weight: 600; }
  nav a.on, nav a:hover { color: var(--fg); }
  nav a.on { border-bottom: 2px solid var(--accent); }
  .inactive { margin-left: auto; font-size: 13px; color: var(--muted); display: flex; gap: 6px; align-items: center; }
  .message { min-height: 20px; font-size: 13px; color: var(--muted); margin: -8px 0 12px; }
  .message.bad { color: var(--bad); }
</style>
