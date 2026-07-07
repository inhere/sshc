import "./styles/app.css";
import { APIError, apiDelete, apiGet, apiPost, apiPut, login, type AuthProfile, type ConfigSummary, type Host, type LogRecord } from "./api";

type ViewName = "hosts" | "auth" | "logs" | "config";

const state = {
  view: "hosts" as ViewName,
  showIP: false,
  message: "",
  error: "",
  needsLogin: false,
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
        <div class="brand">sshc</div>
        <nav class="nav">
          ${navButton("hosts", "Hosts")}
          ${navButton("auth", "Auth")}
          ${navButton("logs", "Logs")}
          ${navButton("config", "Config")}
        </nav>
      </aside>
      <main class="main">
        <header class="toolbar">
          <div>
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
      state.view = button.dataset.view as ViewName;
      state.message = "";
      state.error = "";
      renderShell();
      void loadView();
    });
  }
}

function navButton(view: ViewName, label: string) {
  return `<button class="${state.view === view ? "active" : ""}" data-view="${view}" type="button">${label}</button>`;
}

async function loadView() {
  updateNotice();
  setContent(`<div class="loading">Loading...</div>`);
  try {
    if (state.view === "hosts") await renderHosts();
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
  setTitle("Hosts", "Configured SSH targets");
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
  setContent(`
    <div class="split">
      <form id="host-form" class="editor">
        <h2>Host</h2>
        ${input("name", "Name", true)}
        ${input("ip", "Address", true)}
        ${input("user", "User")}
        ${input("auth_ref", "Auth")}
        ${input("password", "Password", false, "password")}
        ${input("key_path", "Key path")}
        ${input("group", "Group")}
        ${input("port", "Port", false, "number")}
        ${input("jump", "Jump")}
        ${input("remark", "Remark")}
        <div class="form-actions">
          <button type="submit">Save</button>
          <button id="reset-host" type="button">Clear</button>
        </div>
      </form>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Name</th><th>Group</th><th>Address</th><th>Auth</th><th>Remark</th><th></th></tr></thead>
          <tbody>
            ${hosts.map(hostRow).join("") || `<tr><td colspan="6" class="empty">No hosts</td></tr>`}
          </tbody>
        </table>
      </div>
    </div>
  `);
  bindHostForm();
  for (const button of queryAll<HTMLButtonElement>("[data-host-edit]")) {
    button.addEventListener("click", () => fillHostForm(hosts.find((host) => host.name === button.dataset.hostEdit)));
  }
  for (const button of queryAll<HTMLButtonElement>("[data-host-delete]")) {
    button.addEventListener("click", () => void deleteHost(button.dataset.hostDelete || ""));
  }
  for (const button of queryAll<HTMLButtonElement>("[data-host-trust]")) {
    button.addEventListener("click", () => void trustHost(button.dataset.hostTrust || ""));
  }
}

function hostRow(host: Host) {
  const auth = host.auth_ref || host.user || "";
  return `
    <tr>
      <td>${escapeHTML(host.name)}</td>
      <td>${escapeHTML(host.group || "")}</td>
      <td>${escapeHTML([host.ip, host.port ? `:${host.port}` : ""].join(""))}</td>
      <td>${escapeHTML(auth)}</td>
      <td>${escapeHTML(host.remark || "")}</td>
      <td class="row-actions">
        <button data-host-edit="${escapeAttr(host.name)}" type="button">Edit</button>
        <button data-host-trust="${escapeAttr(host.name)}" type="button">Trust</button>
        <button data-host-delete="${escapeAttr(host.name)}" type="button">Delete</button>
      </td>
    </tr>
  `;
}

function bindHostForm() {
  const form = query<HTMLFormElement>("#host-form");
  query("#reset-host").addEventListener("click", () => form.reset());
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const host = formValue<Host>(form);
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
  for (const [key, value] of Object.entries(host)) {
    const input = form.elements.namedItem(key) as HTMLInputElement | null;
    if (input) input.value = value === undefined ? "" : String(value);
  }
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
  setTitle("Auth", "Reusable SSH credentials");
  setActions(`<button id="reload-auth" type="button">Refresh</button>`);
  query("#reload-auth").addEventListener("click", () => void renderAuth());
  const profiles = await apiGet<AuthProfile[]>("/api/auth-profiles");
  setContent(`
    <div class="split">
      <form id="auth-form" class="editor">
        <h2>Auth</h2>
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
        <table>
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
      <td>${escapeHTML(profile.name)}</td>
      <td>${escapeHTML(profile.user || "")}</td>
      <td>${escapeHTML(profile.key_path || "")}</td>
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
  setTitle("Logs", "Recent command execution records");
  setActions(`
    <input id="log-target" class="compact-input" placeholder="target">
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
        <table>
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
  setTitle("Config", "Local sshc configuration");
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

function input(name: string, label: string, required = false, type = "text") {
  return `<label><span>${label}</span><input name="${name}" type="${type}" ${required ? "required" : ""}></label>`;
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
