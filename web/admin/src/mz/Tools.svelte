<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { Tool } from "./types";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  const sections: { kind: Tool["kind"]; title: string; about: string }[] = [
    { kind: "work", title: "Background work", about: "Tasks, goals and schedules: one switch for the whole set." },
    { kind: "native", title: "Her own tools", about: "" },
    { kind: "plugin", title: "Plugins", about: "Switching one on first checks the settings it needs under env: in config.yml." },
    { kind: "mcp", title: "MCP servers", about: "" },
  ];
  let busy = $state("");

  async function flip(t: Tool, box: HTMLInputElement) {
    const on = !t.loaded;
    if (!(on && t.kind === "work" && !confirm("Switch on background work?\n\nPeople can then start tasks, goals and schedules that use the model in the background."))) {
      busy = t.spec;
      await mz.act(() => mz.api.switchTool(t.spec, on), `${t.spec} switched ${on ? "on" : "off"}`);
      busy = "";
    }
    // A refused switch leaves the box showing what is really loaded.
    box.checked = mz.tools.find((x) => x.spec === t.spec)?.loaded ?? t.loaded;
  }

  async function restrict(t: Tool, box: HTMLInputElement) {
    const to = !t.adminOnly;
    busy = t.spec;
    await mz.act(() => mz.api.restrictTool(t.spec, to), `${t.spec} ${to ? "restricted to admins" : "open to everyone"}`);
    busy = "";
    box.checked = mz.tools.find((x) => x.spec === t.spec)?.adminOnly ?? t.adminOnly;
  }

  function resetAll() {
    if (confirm("Put every tool back the way config.yml has it?")) void mz.act(() => mz.api.resetTools(), "tools back to config.yml");
  }
</script>

<Card title="Tools">
  {#snippet actions()}
    {#if mz.tools.some((t) => t.switched)}<button onclick={resetAll}>Reset to config.yml</button>{/if}
  {/snippet}
  <p class="sub">Switching applies now and is kept across restarts. Anything switched here is listed in the banner until
    reset, so a tool turned on for testing isn't left on by mistake.</p>
</Card>

{#each sections as sec (sec.kind)}
  {@const list = mz.tools.filter((t) => t.kind === sec.kind)}
  {#if list.length}
    <Card title={sec.title}>
      {#if sec.about}<p class="sub">{sec.about}</p>{/if}
      <ul class="rows">
        {#each list as t (t.spec)}
          <li>
            <div class="grow">
              <div><strong>{t.spec}</strong>
                {#if t.switched}<Badge text={t.loaded ? "switched on" : "switched off"} tone="busy" />{/if}
                {#if !t.switched && t.inConfig}<Badge text="config.yml" />{/if}
              </div>
              {#if t.description}<div class="sub">{t.description}</div>{/if}
              {#if t.names.length > 1}<div class="sub">loads {t.names.join(", ")}</div>{/if}
            </div>
            {#if t.loaded && t.kind !== "work"}
              <label class="switch" title="Only admins can make her use it (~tools restrict)">
                <input type="checkbox" checked={t.adminOnly} disabled={busy === t.spec} onchange={(e) => restrict(t, e.currentTarget)} />
                admins only
              </label>
            {/if}
            <label class="switch">
              <input type="checkbox" checked={t.loaded} disabled={busy === t.spec} onchange={(e) => flip(t, e.currentTarget)} />
              {t.loaded ? "on" : "off"}
            </label>
          </li>
        {/each}
      </ul>
    </Card>
  {/if}
{/each}

<style>
  p.sub { margin: 0 0 8px; }
  .sub { overflow-wrap: anywhere; }
  .switch { display: flex; gap: 6px; align-items: center; font-size: 13px; min-width: 52px; }
</style>
