import "./styles/app.css";
import { APIError, apiDelete, apiGet, apiPost, apiPut, login, type AuthProfile, type ConfigSummary, type Host, type LogRecord, type TerminalSession } from "./api";
import { mountTerminal, type TerminalMount } from "./terminal";

type ViewName = "hosts" | "auth" | "logs" | "config" | "terminal";
type HostAuthType = "profile" | "pwd" | "keyfile";

const state = {
  view: "hosts" as ViewName,
  showIP: false,
  message: "",
  error: "",
  needsLogin: false,
  terminalHost: "",
  pendingConnectHost: "",
  terminalSessionID: "",
  terminalMount: null as TerminalMount | null,
};

const appRoot = document.querySelector<HTMLDivElement>("#app");
if (!appRoot) {
  throw new Error("missing app root");
}
const app = appRoot;

renderShell();
void loadView();

function renderShell() {
  app.innerHTML = `
    <div class="app-shell">
      <aside class="sidebar">
        <div class="brand"><span class="prompt-mark">$</span><span>sshc</span></div>
        <div class="rail-meta">
          <span>local console</span>
          <strong>known_hosts first</strong>
        </div>
        <nav class="nav">
          ${navButton("hosts", "Hosts", "targets")}
          ${navButton("terminal", "Terminal", "pty")}
          ${navButton("auth", "Auth", "credentials")}
          ${navButton("logs", "Logs", "jsonl")}
          ${navButton("config", "Config", "local")}
        </nav>
      </aside>
      <main class="main">
        <header class="toolbar">
          <div>
            <span id="view-route" class="view-route">sshc://${state.view}</span>
            <h1 id="view-title">Hosts</h1>
            <p id="view-meta"></p>
          </div>
          <div id="view-actions" class="actions"></div>
        </header>
        <div id="notice" class="notice" hidden></div>
        <section id="content" class="content"></section>
      </main>
    </div>
  `;
  for (const button of app.querySelectorAll<HTMLButtonElement>("[data-view]")) {
    button.addEventListener("click", () => {
      disposeTerminal();
      state.view = button.dataset.view as ViewName;
      state.message = "";
      state.error = "";
      renderShell();
      void loadView();
    });
  }
}

function navButton(view: ViewName, label: string, meta: string) {
  return `
    <button class="${state.view === view ? "active" : ""}" data-view="${view}" type="button">
      <span>${label}</span>
      <small>${meta}</small>
    </button>
  `;
}

async function loadView() {
  updateNotice();
  setContent(`<div class="loading">Loading...</div>`);
  try {
    if (state.view === "hosts") await renderHosts();
    if (state.view === "terminal") await renderTerminal();
    if (state.view === "auth") await renderAuth();
    if (state.view === "logs") await renderLogs();
    if (state.view === "config") await renderConfig();
  } catch (err) {
    if (err instanceof APIError && err.status === 401) {
      state.needsLogin = true;
      state.error = "";
      renderLogin();
      return;
    }
    state.error = err instanceof Error ? err.message : String(err);
    updateNotice();
    setContent("");
  }
}

function renderLogin() {
  setTitle("Login", "Access token required");
  setActions("");
  setContent(`
    <form id="login-form" class="login-panel">
      <label><span>Token</span><input name="token" type="password" required autofocus></label>
      <div class="form-actions">
        <button type="submit">Login</button>
      </div>
    </form>
  `);
  query<HTMLFormElement>("#login-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const tokenInput = query<HTMLInputElement>('input[name="token"]');
    try {
      await login(tokenInput.value);
      state.needsLogin = false;
      state.error = "";
      state.message = "";
      await loadView();
    } catch (err) {
      state.error = err instanceof Error ? err.message : String(err);
      updateNotice();
    }
  });
}

function setTitle(title: string, meta = "") {
  text("#view-route", `sshc://${state.view}`);
  text("#view-title", title);
  text("#view-meta", meta);
}

function setActions(html = "") {
  const actions = query("#view-actions");
  actions.innerHTML = html;
}

function setContent(html: string) {
  query("#content").innerHTML = html;
}

function updateNotice() {
  const notice = query("#notice");
  const textValue = state.error || state.message;
  notice.hidden = !textValue;
  notice.className = `notice ${state.error ? "error" : "success"}`;
  notice.textContent = textValue;
}

async function renderHosts() {
  setTitle("Hosts", "Scan targets, connect, trust keys, and keep host records tight.");
  setActions(`
    <label class="toggle"><input id="show-ip" type="checkbox" ${state.showIP ? "checked" : ""}> Show IP</label>
    <button id="reload-hosts" type="button">Refresh</button>
  `);
  query<HTMLInputElement>("#show-ip").addEventListener("change", (event) => {
    state.showIP = (event.target as HTMLInputElement).checked;
    void renderHosts();
  });
  query("#reload-hosts").addEventListener("click", () => void renderHosts());
  const hosts = await apiGet<Host[]>(`/api/hosts${state.showIP ? "?show_ip=1" : ""}`);
  const [rawHosts, profiles] = await Promise.all([apiGet<Host[]>("/api/hosts?show_ip=1"), apiGet<AuthProfile[]>("/api/auth-profiles")]);
  const groups = uniqueSorted(rawHosts.map((host) => host.group || "default"));
  const groupCount = groups.length;
  const jumpCount = rawHosts.filter((host) => host.jump).length;
  setContent(`
    <div class="ops-strip">
      ${metric("Targets", String(hosts.length))}
      ${metric("Groups", String(groupCount))}
      ${metric("Jump routes", String(jumpCount))}
      ${metric("IP mode", state.showIP ? "full" : "masked")}
    </div>
    <div class="split">
      <form id="host-form" class="editor">
        <h2>Host record</h2>
        ${input("name", "Name", true)}
        ${input("ip", "Address", true)}
        ${groupInput(groups)}
        ${authTypeField()}
        <div class="auth-pane" data-auth-pane="profile">
          ${selectField("auth_ref", "Auth profile", profiles.map((profile) => profile.name), false, "Select profile")}
        </div>
        <div class="auth-pane" data-auth-pane="pwd">
          ${input("user", "User")}
          ${input("password", "Password", false, "password")}
        </div>
        <div class="auth-pane" data-auth-pane="keyfile">
          ${input("key_user", "User")}
          ${input("key_path", "Key path")}
        </div>
        ${input("port", "Port", false, "number")}
        ${input("jump", "Jump")}
        ${input("remark", "Remark")}
        <div class="form-actions">
          <button type="submit">Save</button>
          <button id="reset-host" type="button">Clear</button>
        </div>
      </form>
      <div class="table-wrap">
        <table class="resource-table">
          <thead><tr><th>Target</th><th>Group</th><th>Address</th><th>Auth</th><th>Route</th><th></th></tr></thead>
          <tbody>
            ${hosts.map(hostRow).join("") || `<tr><td colspan="6" class="empty">No hosts yet. Add the first target from the form.</td></tr>`}
          </tbody>
        </table>
      </div>
    </div>
  `);
  bindHostForm();
  for (const button of queryAll<HTMLButtonElement>("[data-host-edit]")) {
    button.addEventListener("click", () => fillHostForm(rawHosts.find((host) => host.name === button.dataset.hostEdit)));
  }
  for (const button of queryAll<HTMLButtonElement>("[data-host-delete]")) {
    button.addEventListener("click", () => void deleteHost(button.dataset.hostDelete || ""));
  }
  for (const button of queryAll<HTMLButtonElement>("[data-host-trust]")) {
    button.addEventListener("click", () => void trustHost(button.dataset.hostTrust || ""));
  }
  for (const button of queryAll<HTMLButtonElement>("[data-host-connect]")) {
    button.addEventListener("click", () => void openHostTerminal(button.dataset.hostConnect || ""));
  }
}

function hostRow(host: Host) {
  const auth = host.auth_ref || host.user || "";
  const route = host.jump ? `via ${host.jump}` : host.backend === "command_proxy" ? `via ${host.via || ""}` : "direct";
  const backend = host.backend || "ssh";
  return `
    <tr>
      <td>
        <strong class="host-name">${escapeHTML(host.name)}</strong>
        <span class="subline">${escapeHTML(host.remark || backend)}</span>
      </td>
      <td>${badge(host.group || "default")}</td>
      <td class="mono">${host.ip ? escapeHTML([host.ip, host.port ? `:${host.port}` : ""].join("")) : `<span class="muted">logical</span>`}</td>
      <td>${auth ? badge(auth) : `<span class="muted">inline</span>`}</td>
      <td><span class="route-chip">${escapeHTML(route)}</span></td>
      <td class="row-actions">
        <button data-host-connect="${escapeAttr(host.name)}" type="button">Connect</button>
        <button data-host-trust="${escapeAttr(host.name)}" type="button">Trust</button>
        <button data-host-edit="${escapeAttr(host.name)}" type="button">Edit</button>
        <button data-host-delete="${escapeAttr(host.name)}" type="button">Delete</button>
      </td>
    </tr>
  `;
}

async function openHostTerminal(name: string) {
  disposeTerminal();
  state.view = "terminal";
  state.terminalHost = name;
  state.pendingConnectHost = name;
  state.message = "";
  state.error = "";
  renderShell();
  await loadView();
}

async function renderTerminal() {
  setTitle("Terminal", "Open a browser PTY using saved SSH routes.");
  setActions(`<button id="reload-terminal" type="button">Refresh</button>`);
  query("#reload-terminal").addEventListener("click", () => void renderTerminal());
  const [sessions, hosts] = await Promise.all([apiGet<TerminalSession[]>("/api/terminal/sessions"), apiGet<Host[]>("/api/hosts?show_ip=1")]);
  setContent(`
    <div class="terminal-layout">
      <form id="terminal-form" class="terminal-toolbar">
        ${hostSelectField("host", "Host", hosts, true, "Select host", state.terminalHost)}
        <label><span>Cols</span><input name="cols" type="number" value="120" min="1"></label>
        <label><span>Rows</span><input name="rows" type="number" value="36" min="1"></label>
        <button id="terminal-connect" type="submit">${state.terminalSessionID ? "Reconnect" : "Connect"}</button>
        <button id="terminal-close" type="button" ${state.terminalSessionID ? "" : "disabled"}>Disconnect</button>
      </form>
      <div class="terminal-frame">
        <div class="terminal-bar">
          <span id="terminal-current-host">${escapeHTML(state.terminalHost || "no session")}</span>
          <strong id="terminal-state">${state.terminalSessionID ? "connected" : "idle"}</strong>
        </div>
        <div id="terminal-container" class="terminal-container"></div>
      </div>
      <div class="table-wrap">
        <table>
          <thead><tr><th>ID</th><th>Host</th><th>Started</th><th></th></tr></thead>
          <tbody>${sessions.map(terminalRow).join("") || `<tr><td colspan="4" class="empty">No sessions</td></tr>`}</tbody>
        </table>
      </div>
    </div>
  `);
  query<HTMLFormElement>("#terminal-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const values = formValue<{ host?: string; cols?: string; rows?: string }>(event.currentTarget as HTMLFormElement);
    await connectTerminal(String(values.host || ""), Number(values.cols || 120), Number(values.rows || 36));
  });
  query("#terminal-close").addEventListener("click", () => void closeTerminal(state.terminalSessionID));
  for (const button of queryAll<HTMLButtonElement>("[data-terminal-close]")) {
    button.addEventListener("click", () => void closeTerminal(button.dataset.terminalClose || ""));
  }
  if (state.pendingConnectHost) {
    const host = state.pendingConnectHost;
    state.pendingConnectHost = "";
    await connectTerminal(host, 120, 36);
  }
}

function terminalRow(session: TerminalSession) {
  return `
    <tr>
      <td class="mono">${escapeHTML(session.id)}</td>
      <td>${escapeHTML(session.host)}</td>
      <td>${escapeHTML(session.started_at || "")}</td>
      <td class="row-actions"><button data-terminal-close="${escapeAttr(session.id)}" type="button">Close</button></td>
    </tr>
  `;
}

async function connectTerminal(host: string, cols: number, rows: number) {
  host = host.trim();
  if (!host) return;
  disposeTerminal();
  try {
    const session = await apiPost<TerminalSession>("/api/terminal/sessions", { host, cols, rows, term: "xterm-256color" });
    state.terminalHost = host;
    state.terminalSessionID = session.id;
    updateTerminalStatus("connected", session.host, true);
    const container = query("#terminal-container");
    state.terminalMount = mountTerminal(
      container,
      session.id,
      (nextCols, nextRows) => {
        void apiPost(`/api/terminal/sessions/${encodeURIComponent(session.id)}/resize`, { cols: nextCols, rows: nextRows }).catch(() => {});
      },
      () => markTerminalDisconnected(session.id, session.host),
    );
    state.terminalMount.terminal.writeln(`connected to ${session.host}`);
    state.error = "";
    updateNotice();
  } catch (err) {
    state.error = err instanceof Error ? err.message : String(err);
    updateNotice();
  }
}

async function closeTerminal(id: string) {
  if (!id) return;
  try {
    await apiDelete(`/api/terminal/sessions/${encodeURIComponent(id)}`);
    disposeTerminal();
    if (state.terminalSessionID === id) state.terminalSessionID = "";
    state.message = `closed terminal ${id}`;
    state.error = "";
    await renderTerminal();
  } catch (err) {
    state.error = err instanceof Error ? err.message : String(err);
    updateNotice();
  }
}

function disposeTerminal() {
  if (state.terminalMount) {
    state.terminalMount.dispose();
    state.terminalMount = null;
  }
}

function markTerminalDisconnected(id: string, host: string) {
  if (state.terminalSessionID !== id) return;
  state.terminalSessionID = "";
  updateTerminalStatus("idle", host, false);
}

function bindHostForm() {
  const form = query<HTMLFormElement>("#host-form");
  bindHostAuthType(form);
  query("#reset-host").addEventListener("click", () => {
    form.reset();
    form.dataset.mode = "";
    setHostAuthType(form, "profile");
  });
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const host = hostFormValue(form);
    if (host.port) host.port = Number(host.port);
    try {
      const existing = form.dataset.mode === "edit";
      if (existing) await apiPut<Host>(`/api/hosts/${encodeURIComponent(host.name)}`, host);
      else await apiPost<Host>("/api/hosts", host);
      state.message = `saved host ${host.name}`;
      state.error = "";
      await renderHosts();
    } catch (err) {
      state.error = err instanceof Error ? err.message : String(err);
      updateNotice();
    }
  });
}

function fillHostForm(host?: Host) {
  if (!host) return;
  const form = query<HTMLFormElement>("#host-form");
  form.dataset.mode = "edit";
  setHostAuthType(form, hostAuthType(host));
  for (const [key, value] of Object.entries(host)) {
    const input = form.elements.namedItem(key) as HTMLInputElement | null;
    if (input) input.value = value === undefined ? "" : String(value);
  }
  const keyUserInput = form.elements.namedItem("key_user") as HTMLInputElement | null;
  if (keyUserInput) keyUserInput.value = host.user || "";
}

async function deleteHost(name: string) {
  if (!name || !confirm(`Delete host ${name}?`)) return;
  await apiDelete(`/api/hosts/${encodeURIComponent(name)}`);
  state.message = `deleted host ${name}`;
  state.error = "";
  await renderHosts();
}

async function trustHost(name: string) {
  if (!name) return;
  try {
    await apiPost(`/api/hosts/${encodeURIComponent(name)}/trust`);
    state.message = `trusted host ${name}`;
    state.error = "";
  } catch (err) {
    state.error = err instanceof Error ? err.message : String(err);
  }
  updateNotice();
}

async function renderAuth() {
  setTitle("Auth", "Reusable SSH identities without exposing saved secrets.");
  setActions(`<button id="reload-auth" type="button">Refresh</button>`);
  query("#reload-auth").addEventListener("click", () => void renderAuth());
  const profiles = await apiGet<AuthProfile[]>("/api/auth-profiles");
  setContent(`
    <div class="split">
      <form id="auth-form" class="editor">
        <h2>Credential profile</h2>
        ${input("name", "Name", true)}
        ${input("user", "User")}
        ${input("password", "Password", false, "password")}
        ${input("key_path", "Key path")}
        ${input("remark", "Remark")}
        <div class="form-actions">
          <button type="submit">Save</button>
          <button id="reset-auth" type="button">Clear</button>
        </div>
      </form>
      <div class="table-wrap">
        <table class="resource-table">
          <thead><tr><th>Name</th><th>User</th><th>Key</th><th>Remark</th><th></th></tr></thead>
          <tbody>${profiles.map(authRow).join("") || `<tr><td colspan="5" class="empty">No auth profiles</td></tr>`}</tbody>
        </table>
      </div>
    </div>
  `);
  bindAuthForm();
  for (const button of queryAll<HTMLButtonElement>("[data-auth-edit]")) {
    button.addEventListener("click", () => fillAuthForm(profiles.find((profile) => profile.name === button.dataset.authEdit)));
  }
  for (const button of queryAll<HTMLButtonElement>("[data-auth-delete]")) {
    button.addEventListener("click", () => void deleteAuth(button.dataset.authDelete || ""));
  }
}

function authRow(profile: AuthProfile) {
  return `
    <tr>
      <td><strong class="host-name">${escapeHTML(profile.name)}</strong></td>
      <td>${escapeHTML(profile.user || "")}</td>
      <td class="mono">${escapeHTML(profile.key_path || "")}</td>
      <td>${escapeHTML(profile.remark || "")}</td>
      <td class="row-actions">
        <button data-auth-edit="${escapeAttr(profile.name)}" type="button">Edit</button>
        <button data-auth-delete="${escapeAttr(profile.name)}" type="button">Delete</button>
      </td>
    </tr>
  `;
}

function bindAuthForm() {
  const form = query<HTMLFormElement>("#auth-form");
  query("#reset-auth").addEventListener("click", () => form.reset());
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const profile = formValue<AuthProfile>(form);
    try {
      if (form.dataset.mode === "edit") await apiPut<AuthProfile>(`/api/auth-profiles/${encodeURIComponent(profile.name)}`, profile);
      else await apiPost<AuthProfile>("/api/auth-profiles", profile);
      state.message = `saved auth ${profile.name}`;
      state.error = "";
      await renderAuth();
    } catch (err) {
      state.error = err instanceof Error ? err.message : String(err);
      updateNotice();
    }
  });
}

function fillAuthForm(profile?: AuthProfile) {
  if (!profile) return;
  const form = query<HTMLFormElement>("#auth-form");
  form.dataset.mode = "edit";
  for (const [key, value] of Object.entries(profile)) {
    const input = form.elements.namedItem(key) as HTMLInputElement | null;
    if (input) input.value = value === undefined ? "" : String(value);
  }
}

async function deleteAuth(name: string) {
  if (!name || !confirm(`Delete auth ${name}?`)) return;
  try {
    await apiDelete(`/api/auth-profiles/${encodeURIComponent(name)}`);
    state.message = `deleted auth ${name}`;
    state.error = "";
    await renderAuth();
  } catch (err) {
    state.error = err instanceof Error ? err.message : String(err);
    updateNotice();
  }
}

async function renderLogs() {
  setTitle("Logs", "Read JSONL run history by target, text match, or task output.");
  const hosts = await apiGet<Host[]>("/api/hosts?show_ip=1");
  setActions(`
    <select id="log-target" class="compact-input host-select">
      <option value="">All hosts</option>
      ${hosts.map((host) => `<option value="${escapeAttr(host.name)}">${escapeHTML(hostOptionLabel(host))}</option>`).join("")}
    </select>
    <input id="log-match" class="compact-input" placeholder="match">
    <button id="reload-logs" type="button">Search</button>
  `);
  const load = async () => {
    const target = query<HTMLInputElement>("#log-target").value.trim();
    const match = query<HTMLInputElement>("#log-match").value.trim();
    const params = new URLSearchParams({ tail: "50" });
    if (target) params.set("target", target);
    if (match) params.set("match", match);
    const records = await apiGet<LogRecord[]>(`/api/logs?${params.toString()}`);
    setContent(`
      <div class="table-wrap">
        <table class="resource-table">
          <thead><tr><th>Time</th><th>Host</th><th>Status</th><th>Command</th><th>Duration</th><th></th></tr></thead>
          <tbody>${records.map(logRow).join("") || `<tr><td colspan="6" class="empty">No logs</td></tr>`}</tbody>
        </table>
      </div>
      <pre id="log-output" class="output" hidden></pre>
    `);
    for (const button of queryAll<HTMLButtonElement>("[data-log-output]")) {
      button.addEventListener("click", () => void showLogOutput(button.dataset.logOutput || ""));
    }
  };
  query("#reload-logs").addEventListener("click", () => void load());
  await load();
}

function logRow(record: LogRecord) {
  const id = String(record.task_id || "");
  return `
    <tr>
      <td>${escapeHTML(String(record.time || record.started_at || ""))}</td>
      <td>${escapeHTML(String(record.host || record.target || ""))}</td>
      <td><span class="status ${record.status === "success" ? "ok" : "bad"}">${escapeHTML(String(record.status || ""))}</span></td>
      <td class="mono">${escapeHTML(String(record.command || ""))}</td>
      <td>${escapeHTML(String(record.duration_ms ?? ""))}</td>
      <td class="row-actions">${id ? `<button data-log-output="${escapeAttr(id)}" type="button">Output</button>` : ""}</td>
    </tr>
  `;
}

async function showLogOutput(taskID: string) {
  const output = query<HTMLPreElement>("#log-output");
  const data = await apiGet<{ output: string }>(`/api/logs/${encodeURIComponent(taskID)}/output?tail=200`);
  output.hidden = false;
  output.textContent = data.output || "";
}

async function renderConfig() {
  setTitle("Config", "Local paths, defaults, and config doctor results.");
  setActions(`<button id="reload-config" type="button">Refresh</button>`);
  query("#reload-config").addEventListener("click", () => void renderConfig());
  const summary = await apiGet<ConfigSummary>("/api/config/summary");
  setContent(`
    <div class="summary-grid">
      ${summaryItem("Config", summary.path)}
      ${summaryItem("Logs", summary.logs_path || "(default)")}
      ${summaryItem("Hosts", String(summary.host_count))}
      ${summaryItem("Auth", String(summary.auth_count))}
      ${summaryItem("Readonly", summary.readonly ? "yes" : "no")}
      ${summaryItem("Doctor", summary.doctor_ok ? "ok" : "issues")}
    </div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>Level</th><th>Item</th><th>Message</th></tr></thead>
        <tbody>${summary.doctor.map((issue) => `<tr><td>${escapeHTML(issue.level)}</td><td>${escapeHTML(issue.item)}</td><td>${escapeHTML(issue.message)}</td></tr>`).join("")}</tbody>
      </table>
    </div>
  `);
}

function summaryItem(label: string, value: string) {
  return `<div class="summary-item"><span>${escapeHTML(label)}</span><strong>${escapeHTML(value)}</strong></div>`;
}

function metric(label: string, value: string) {
  return `<div class="metric"><span>${escapeHTML(label)}</span><strong>${escapeHTML(value)}</strong></div>`;
}

function badge(value: string) {
  return `<span class="badge">${escapeHTML(value)}</span>`;
}

function input(name: string, label: string, required = false, type = "text") {
  return `<label><span>${label}</span><input name="${name}" type="${type}" ${required ? "required" : ""}></label>`;
}

function groupInput(groups: string[]) {
  return `
    <label><span>Group</span><input name="group" list="host-groups" placeholder="default"></label>
    <datalist id="host-groups">
      ${groups.map((group) => `<option value="${escapeAttr(group)}"></option>`).join("")}
    </datalist>
  `;
}

function selectField(name: string, label: string, values: string[], required = false, placeholder = "Select", selected = "") {
  return `
    <label><span>${label}</span>
      <select name="${name}" ${required ? "required" : ""}>
        <option value="">${escapeHTML(placeholder)}</option>
        ${values.map((value) => `<option value="${escapeAttr(value)}" ${value === selected ? "selected" : ""}>${escapeHTML(value)}</option>`).join("")}
      </select>
    </label>
  `;
}

function hostSelectField(name: string, label: string, hosts: Host[], required = false, placeholder = "Select host", selected = "") {
  return `
    <label><span>${label}</span>
      <select name="${name}" ${required ? "required" : ""}>
        <option value="">${escapeHTML(placeholder)}</option>
        ${hosts.map((host) => `<option value="${escapeAttr(host.name)}" ${host.name === selected ? "selected" : ""}>${escapeHTML(hostOptionLabel(host))}</option>`).join("")}
      </select>
    </label>
  `;
}

function hostOptionLabel(host: Host) {
  const details = [maskHostIP(host.ip || ""), host.remark || ""].map((value) => value.trim()).filter(Boolean);
  if (details.length === 0) {
    return host.name;
  }
  return `${host.name} (${details.join("/")})`;
}

function maskHostIP(ip: string) {
  const parts = ip.split(".");
  if (parts.length !== 4 || parts.some((part) => part === "")) {
    return ip;
  }
  return `${parts[0]}.*.*.${parts[3]}`;
}

function authTypeField() {
  return `
    <fieldset class="segmented auth-type">
      <legend>Auth type</legend>
      <label><input type="radio" name="auth_type" value="profile" checked><span>Profile</span></label>
      <label><input type="radio" name="auth_type" value="pwd"><span>Password</span></label>
      <label><input type="radio" name="auth_type" value="keyfile"><span>Key file</span></label>
    </fieldset>
  `;
}

function bindHostAuthType(form: HTMLFormElement) {
  for (const radio of form.querySelectorAll<HTMLInputElement>('input[name="auth_type"]')) {
    radio.addEventListener("change", () => updateHostAuthPanes(form));
  }
  updateHostAuthPanes(form);
}

function setHostAuthType(form: HTMLFormElement, type: HostAuthType) {
  const input = form.querySelector<HTMLInputElement>(`input[name="auth_type"][value="${type}"]`);
  if (input) input.checked = true;
  updateHostAuthPanes(form);
}

function updateHostAuthPanes(form: HTMLFormElement) {
  const type = selectedHostAuthType(form);
  for (const pane of form.querySelectorAll<HTMLElement>("[data-auth-pane]")) {
    pane.hidden = pane.dataset.authPane !== type;
  }
}

function selectedHostAuthType(form: HTMLFormElement): HostAuthType {
  const data = new FormData(form);
  const value = String(data.get("auth_type") || "profile");
  return value === "pwd" || value === "keyfile" ? value : "profile";
}

function hostAuthType(host: Host): HostAuthType {
  if (host.auth_ref) return "profile";
  if (host.key_path) return "keyfile";
  return "pwd";
}

function hostFormValue(form: HTMLFormElement): Host {
  const type = selectedHostAuthType(form);
  const host = formValue<Host>(form);
  delete (host as Host & { auth_type?: string }).auth_type;
  if (type === "profile") {
    delete host.user;
    delete host.password;
    delete host.key_path;
  } else if (type === "pwd") {
    delete host.auth_ref;
    delete host.key_path;
  } else {
    host.user = String(new FormData(form).get("key_user") || "").trim();
    delete host.auth_ref;
    delete host.password;
  }
  delete (host as Host & { key_user?: string }).key_user;
  return host;
}

function updateTerminalStatus(status: "idle" | "connected", host: string, connected: boolean) {
  const statusNode = app.querySelector<HTMLElement>("#terminal-state");
  const hostNode = app.querySelector<HTMLElement>("#terminal-current-host");
  const closeButton = app.querySelector<HTMLButtonElement>("#terminal-close");
  const connectButton = app.querySelector<HTMLButtonElement>("#terminal-connect");
  if (statusNode) statusNode.textContent = status;
  if (hostNode) hostNode.textContent = host || "no session";
  if (closeButton) closeButton.disabled = !connected;
  if (connectButton) connectButton.textContent = connected ? "Reconnect" : "Connect";
}

function uniqueSorted(values: string[]) {
  return Array.from(new Set(values.map((value) => value.trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b));
}

function formValue<T>(form: HTMLFormElement): T {
  const data = new FormData(form);
  const obj: Record<string, FormDataEntryValue> = {};
  for (const [key, value] of data.entries()) {
    if (String(value).trim() !== "") obj[key] = value;
  }
  return obj as T;
}

function query<T extends Element = HTMLElement>(selector: string): T {
  const node = app.querySelector<T>(selector);
  if (!node) throw new Error(`missing ${selector}`);
  return node;
}

function queryAll<T extends Element = HTMLElement>(selector: string): T[] {
  return Array.from(app.querySelectorAll<T>(selector));
}

function text(selector: string, value: string) {
  query(selector).textContent = value;
}

function escapeHTML(value: string) {
  return value.replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char] || char);
}

function escapeAttr(value: string) {
  return escapeHTML(value);
}
