'use strict';

// ---- low-level API helper ---------------------------------------------------

async function api(method, path, body) {
  const opts = { method, credentials: 'same-origin', headers: {} };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  const text = await res.text();
  let data = null;
  if (text) { try { data = JSON.parse(text); } catch { data = { error: text }; } }
  if (!res.ok) {
    const err = new Error((data && data.error) || ('request failed: ' + res.status));
    err.data = data;
    err.status = res.status;
    throw err;
  }
  return data;
}

// ---- small helpers -----------------------------------------------------------

function escapeHTML(s) {
  const d = document.createElement('div');
  d.textContent = s == null ? '' : String(s);
  return d.innerHTML;
}
function icon(name, extra) { return `<i class="fa ${name} ${extra || ''}"></i>`; }
function humanMB(n) {
  if (n === null || n === undefined || isNaN(n)) return '-';
  if (n < 1024) return `${n} MB`;
  return `${(n / 1024).toFixed(1)} GB`;
}
// fetchTenantNames/fetchRepoNames back every tenant/repo picker in the UI —
// dropdowns sourced from what actually exists, rather than free-text
// fields an operator has to spell correctly by hand. A tenant session
// only ever has itself to pick from; admin gets the real list.
async function fetchTenantNames() {
  if (!me.admin) return [me.username];
  const users = (await api('GET', '/api/users')) || [];
  return users.map((u) => u.username);
}
async function fetchRepoNames(tenant) {
  const qs = tenant ? '?tenant=' + encodeURIComponent(tenant) : '';
  const repos = (await api('GET', '/api/repos' + qs)) || [];
  return repos.map((r) => r.Name);
}
function optionsHTML(values, emptyLabel) {
  if (values.length === 0) return `<option value="">${escapeHTML(emptyLabel || 'none yet')}</option>`;
  return values.map((v) => `<option value="${escapeHTML(v)}">${escapeHTML(v)}</option>`).join('');
}

function badgeClass(status) {
  if (status === 'ok') return 'badge-ok';
  if (status === 'warn') return 'badge-warn';
  if (status === 'fail') return 'badge-fail';
  return 'badge-off';
}

const CHART_COLORS = {
  indigo: '#6366f1', emerald: '#10b981', amber: '#f59e0b', rose: '#f43f5e',
  sky: '#0ea5e9', slate: '#94a3b8', violet: '#8b5cf6', teal: '#14b8a6',
};
if (typeof Chart !== 'undefined') {
  Chart.defaults.font.family = "-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif";
  Chart.defaults.font.size = 12;
  Chart.defaults.color = '#64748b';
  Chart.defaults.plugins.legend.labels.boxWidth = 12;
  Chart.defaults.plugins.legend.labels.usePointStyle = true;
}

// ---- toast notifications ------------------------------------------------------

function toast(message, kind) {
  const wrap = document.getElementById('toast-container');
  const el = document.createElement('div');
  el.className = 'toast' + (kind ? ' ' + kind : '');
  el.textContent = message;
  wrap.appendChild(el);
  setTimeout(() => el.remove(), 6000);
}

// ---- confirmation / prompt modal ----------------------------------------------

// Resolves to `false` on cancel, or `{ ok: true, values }` on confirm —
// `values` maps every [id] element inside `body` to its value at the moment
// OK was clicked. Used both for destructive-action confirmation and as a
// lightweight form (see promptSetPassword, the CreateRepo/AddKey forms use
// their own inline forms instead since those stay on the page).
function confirmModal({ title, body, confirmLabel, danger, requireText }) {
  return new Promise((resolve) => {
    const backdrop = document.createElement('div');
    backdrop.className = 'modal-backdrop';
    backdrop.innerHTML = `
      <div class="modal-box">
        <h3 class="text-base font-semibold text-slate-900 mb-2">${escapeHTML(title)}</h3>
        <div class="text-sm text-slate-600">${body}</div>
        ${requireText ? `<label class="field-label mt-3">Type <code class="chip">${escapeHTML(requireText)}</code> to confirm
          <input type="text" id="modal-confirm-text" autocomplete="off" class="field-input mt-1"></label>` : ''}
        <div class="flex justify-end gap-2 mt-5">
          <button id="modal-cancel" class="btn">Cancel</button>
          <button id="modal-ok" class="btn ${danger ? 'btn-danger-solid' : 'btn-primary'}">${escapeHTML(confirmLabel || 'Confirm')}</button>
        </div>
      </div>`;
    document.body.appendChild(backdrop);
    const okBtn = backdrop.querySelector('#modal-ok');
    const input = backdrop.querySelector('#modal-confirm-text');
    if (requireText) {
      okBtn.disabled = true;
      input.addEventListener('input', () => { okBtn.disabled = input.value !== requireText; });
    }
    backdrop.querySelector('#modal-cancel').onclick = () => { backdrop.remove(); resolve(false); };
    okBtn.onclick = () => {
      const values = {};
      backdrop.querySelectorAll('[id]').forEach((el) => {
        if (el.tagName === 'INPUT' && el.type === 'checkbox') values[el.id] = el.checked;
        else if ('value' in el) values[el.id] = el.value;
      });
      backdrop.remove();
      resolve({ ok: true, values });
    };
  });
}

// Set or reset a login password — the fix for a real gap: creating an
// account previously had no follow-up way to give it a usable password.
// Blank means "generate one server-side", shown back exactly once.
async function promptSetPassword(username, isSelf) {
  const res = await confirmModal({
    title: isSelf ? 'Change your password' : `Set password for ${username}`,
    body: `<p class="text-xs text-slate-500 mb-2">Leave blank to generate a strong one — shown once, right after.</p>
           <input type="password" id="pw-value" class="field-input" placeholder="New password (optional)" autocomplete="new-password">`,
    confirmLabel: 'Set password',
  });
  if (!res) return;
  try {
    const body = { password: res.values['pw-value'] };
    if (!isSelf) body.username = username;
    const result = await api('POST', '/api/passwd', body);
    if (result && result.generated_password) {
      await confirmModal({
        title: 'Password generated',
        body: `<p class="text-sm mb-2">Save this now — it will not be shown again:</p>
               <div class="chip block w-full break-all select-all">${escapeHTML(result.generated_password)}</div>`,
        confirmLabel: 'Done',
      });
    } else {
      toast('Password updated', 'ok');
    }
  } catch (err) {
    toast('Could not set password: ' + err.message, 'err');
  }
}

// ---- session / login ----------------------------------------------------------

let me = null; // { username, admin }

function showLogin() {
  document.getElementById('login-view').hidden = false;
  document.getElementById('app-view').hidden = true;
}

function showApp() {
  document.getElementById('login-view').hidden = true;
  document.getElementById('app-view').hidden = false;
  document.getElementById('whoami').textContent = me.username + (me.admin ? ' (admin)' : '');
  document.querySelectorAll('.admin-only').forEach((n) => { n.hidden = !me.admin; });
  switchView('dashboard');
}

document.getElementById('login-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const errBox = document.getElementById('login-error');
  errBox.textContent = '';
  try {
    me = await api('POST', '/api/login', {
      username: document.getElementById('login-username').value,
      password: document.getElementById('login-password').value,
    });
    document.getElementById('login-password').value = '';
    showApp();
  } catch (err) {
    errBox.textContent = err.message;
  }
});

document.getElementById('logout-btn').addEventListener('click', async () => {
  await api('POST', '/api/logout');
  me = null;
  showLogin();
});

document.getElementById('change-password-btn').addEventListener('click', () => {
  promptSetPassword(me.username, true);
});

// ---- navigation -----------------------------------------------------------------

const views = {};
let currentCleanup = null;

function switchView(name, param) {
  if (currentCleanup) { currentCleanup(); currentCleanup = null; }
  document.querySelectorAll('.nav-item[data-view]').forEach((btn) => {
    btn.classList.toggle('active', btn.dataset.view === name);
  });
  const container = document.getElementById('content');
  container.innerHTML = '<p class="text-slate-400 text-sm">Loading…</p>';
  const fn = views[name];
  if (!fn) { container.innerHTML = '<p>Unknown view.</p>'; return; }
  Promise.resolve(fn(container, param)).then((cleanup) => {
    if (typeof cleanup === 'function') currentCleanup = cleanup;
  }).catch((err) => {
    if (err.status === 401) { showLogin(); return; }
    container.innerHTML = `<div class="card"><p class="text-rose-600 text-sm">${escapeHTML(err.message)}</p></div>`;
  });
}

document.querySelectorAll('.nav-item[data-view]').forEach((btn) => {
  btn.addEventListener('click', () => switchView(btn.dataset.view));
});

// ---- Dashboard ------------------------------------------------------------------

views.dashboard = async (container) => {
  const repos = (await api('GET', '/api/repos')) || [];
  const totalMB = repos.reduce((sum, r) => sum + (r.SizeMB || 0), 0);
  const initialized = repos.filter((r) => r.Initialized).length;
  const byTenant = {};
  repos.forEach((r) => { byTenant[r.Tenant] = (byTenant[r.Tenant] || 0) + (r.SizeMB || 0); });
  const tenants = Object.keys(byTenant);

  container.innerHTML = `
    <h2 class="page-title">${icon('fa-tachometer', 'text-indigo-500')}Dashboard</h2>
    <div class="grid grid-cols-1 md:grid-cols-3 gap-5 mb-1">
      <div class="card">
        <h3 class="card-title">${icon('fa-database')}Repositories</h3>
        <div class="text-3xl font-semibold text-slate-900">${repos.length}</div>
        <div class="text-xs text-slate-500 mt-1">${initialized} initialized, ${repos.length - initialized} reserved</div>
      </div>
      <div class="card">
        <h3 class="card-title">${icon('fa-hdd-o')}Storage used</h3>
        <div class="text-3xl font-semibold text-slate-900">${humanMB(totalMB)}</div>
        <div class="text-xs text-slate-500 mt-1">across ${tenants.length || 0} tenant(s)</div>
      </div>
      <div class="card">
        <h3 class="card-title">${icon('fa-user-circle-o')}Signed in as</h3>
        <div class="text-xl font-semibold text-slate-900">${escapeHTML(me.username)}</div>
        <div class="text-xs text-slate-500 mt-1">${me.admin ? 'Administrator — sees every tenant' : 'Tenant — sees your own repositories only'}</div>
      </div>
    </div>
    ${tenants.length > 0 ? `<div class="card">
      <h3 class="card-title">${icon('fa-pie-chart')}Storage by tenant</h3>
      <div class="h-64"><canvas id="chart-tenants"></canvas></div>
    </div>` : `<div class="card"><p class="text-sm text-slate-500">No repositories yet. Create one from the Repositories tab.</p></div>`}
  `;

  if (tenants.length > 0 && typeof Chart !== 'undefined') {
    const palette = [CHART_COLORS.indigo, CHART_COLORS.emerald, CHART_COLORS.amber, CHART_COLORS.rose, CHART_COLORS.sky, CHART_COLORS.violet, CHART_COLORS.teal, CHART_COLORS.slate];
    const chart = new Chart(document.getElementById('chart-tenants'), {
      type: 'doughnut',
      data: {
        labels: tenants,
        datasets: [{ data: tenants.map((t) => byTenant[t]), backgroundColor: tenants.map((_, i) => palette[i % palette.length]), borderWidth: 0 }],
      },
      options: {
        maintainAspectRatio: false,
        plugins: { legend: { position: 'right' }, tooltip: { callbacks: { label: (ctx) => `${ctx.label}: ${humanMB(ctx.raw)}` } } },
      },
    });
    return () => chart.destroy();
  }
};

// ---- Repositories ---------------------------------------------------------------

views.repos = async (container) => {
  const tenants = await fetchTenantNames();

  container.innerHTML = `
    <h2 class="page-title">${icon('fa-database', 'text-indigo-500')}Repositories</h2>
    <div class="card">
      <div class="flex items-center justify-between mb-3">
        <h3 class="card-title mb-0">${icon('fa-list')}All repositories</h3>
        ${me.admin ? `<div class="flex items-center gap-2">
          <label class="field-label mb-0">Tenant</label>
          <select id="repo-filter-tenant" class="field-input" style="width:auto">
            <option value="">All tenants</option>
            ${optionsHTML(tenants)}
          </select>
        </div>` : ''}
      </div>
      <table class="data-table">
        <thead><tr><th>Tenant</th><th>Name</th><th>Status</th><th>Created</th><th></th></tr></thead>
        <tbody id="repo-rows"></tbody>
      </table>
    </div>
    <div class="card">
      <h3 class="card-title">${icon('fa-plus-circle')}Create a repository</h3>
      <form id="repo-create-form" class="flex flex-wrap items-end gap-3">
        ${me.admin ? `<div><label class="field-label">Tenant</label>
          <select id="repo-new-tenant" class="field-input" required>${optionsHTML(tenants, 'no tenants yet')}</select></div>` : ''}
        <div><label class="field-label">Repository name</label><input type="text" id="repo-new-name" class="field-input" required></div>
        <button type="submit" class="btn btn-primary">${icon('fa-plus')}<span>Create</span></button>
      </form>
      <p id="repo-create-hint" class="text-xs text-slate-500 mt-3"></p>
    </div>`;

  async function loadRepos() {
    const tenantFilter = me.admin ? document.getElementById('repo-filter-tenant').value : '';
    const qs = tenantFilter ? '?tenant=' + encodeURIComponent(tenantFilter) : '';
    const repos = (await api('GET', '/api/repos' + qs)) || [];
    const tbody = document.getElementById('repo-rows');
    if (repos.length === 0) {
      tbody.innerHTML = `<tr><td colspan="5" class="text-slate-400 text-center py-4">No repositories yet.</td></tr>`;
      return;
    }
    tbody.innerHTML = repos.map((r) => `
      <tr>
        <td>${escapeHTML(r.Tenant)}</td>
        <td class="font-medium text-slate-900">${escapeHTML(r.Name)}</td>
        <td>${r.Initialized
          ? `<span class="badge badge-ok">${icon('fa-check-circle', 'mr-0.5')}${humanMB(r.SizeMB)}</span>`
          : `<span class="badge badge-off">not initialized</span>`}
          ${r.Disabled ? `<span class="badge badge-warn ml-1">${icon('fa-ban', 'mr-0.5')}disabled</span>` : ''}</td>
        <td class="text-slate-500">${escapeHTML((r.CreatedAt || '').slice(0, 10))}</td>
        <td class="text-right space-x-3">
          <button class="rowbtn text-indigo-600" data-action="${r.Disabled ? 'enable' : 'disable'}" data-tenant="${escapeHTML(r.Tenant)}" data-name="${escapeHTML(r.Name)}">${r.Disabled ? 'enable' : 'disable'}</button>
          <button class="rowbtn" data-action="delete" data-tenant="${escapeHTML(r.Tenant)}" data-name="${escapeHTML(r.Name)}">delete</button>
        </td>
      </tr>`).join('');

    tbody.querySelectorAll('button[data-action="delete"]').forEach((btn) => {
      btn.addEventListener('click', async () => {
        const t = btn.dataset.tenant, n = btn.dataset.name;
        const res = await confirmModal({
          title: 'Move to trash?',
          body: `<p>${escapeHTML(t)}/${escapeHTML(n)} will be moved to the trash — recoverable until purged. This action itself is not shown here yet in the web UI; ask an admin to purge from the CLI if you need it gone permanently.</p>`,
          confirmLabel: 'Delete',
          danger: true,
        });
        if (!res) return;
        try {
          await api('DELETE', `/api/repos/${encodeURIComponent(t)}/${encodeURIComponent(n)}`);
          toast('Moved to trash', 'ok');
          loadRepos();
        } catch (err) {
          toast('Delete failed: ' + err.message, 'err');
        }
      });
    });
    tbody.querySelectorAll('button[data-action="disable"], button[data-action="enable"]').forEach((btn) => {
      btn.addEventListener('click', async () => {
        const t = btn.dataset.tenant, n = btn.dataset.name, action = btn.dataset.action;
        try {
          await api('POST', `/api/repos/${encodeURIComponent(t)}/${encodeURIComponent(n)}/${action}`);
          toast(action === 'disable' ? 'Repository disabled — every key restricted to it is locked out' : 'Repository enabled', 'ok');
          loadRepos();
        } catch (err) {
          toast(`Could not ${action}: ` + err.message, 'err');
        }
      });
    });
  }

  if (me.admin) {
    document.getElementById('repo-filter-tenant').addEventListener('change', loadRepos);
  }
  document.getElementById('repo-create-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const hint = document.getElementById('repo-create-hint');
    hint.textContent = '';
    const body = { name: document.getElementById('repo-new-name').value.trim() };
    if (me.admin) body.tenant = document.getElementById('repo-new-tenant').value;
    try {
      const repo = await api('POST', '/api/repos', body);
      document.getElementById('repo-new-name').value = '';
      hint.innerHTML = `Reserved. From the tenant's own machine: <code class="chip">borg init --encryption=repokey-blake2 ssh://${escapeHTML(repo.Tenant)}@&lt;this-host&gt;:&lt;borg-port&gt;${escapeHTML(repo.Path)}</code>`;
      toast('Repository created', 'ok');
      loadRepos();
    } catch (err) {
      hint.textContent = err.message;
    }
  });

  await loadRepos();
};

// ---- SSH keys ---------------------------------------------------------------

views.keys = async (container) => {
  const tenants = await fetchTenantNames();

  container.innerHTML = `
    <h2 class="page-title">${icon('fa-key', 'text-indigo-500')}SSH Keys</h2>
    <div class="card">
      <div class="flex items-center justify-between mb-3">
        <h3 class="card-title mb-0">${icon('fa-list')}Registered keys</h3>
        ${me.admin ? `<div class="flex items-center gap-2">
          <label class="field-label mb-0">Tenant</label>
          <select id="keys-filter-tenant" class="field-input" style="width:auto">${optionsHTML(tenants, 'no tenants yet')}</select>
        </div>` : ''}
      </div>
      <table class="data-table">
        <thead><tr><th>Repository</th><th>Mode</th><th>Comment</th><th></th></tr></thead>
        <tbody id="keys-rows"></tbody>
      </table>
    </div>
    <div class="card">
      <h3 class="card-title">${icon('fa-plus-circle')}Register a key</h3>
      <p class="text-xs text-slate-500 mb-3">A registered key can only run <code class="chip">borg serve</code> against the one repository it names — never a shell, never another tenant's data.</p>
      <form id="key-add-form">
        <div class="flex flex-wrap items-end gap-3 mb-3">
          ${me.admin ? `<div><label class="field-label">Tenant</label>
            <select id="key-tenant" class="field-input" required>${optionsHTML(tenants, 'no tenants yet')}</select></div>` : ''}
          <div><label class="field-label">Repository</label><select id="key-repo" class="field-input" required></select></div>
          <label class="field-checkbox-row"><input type="checkbox" id="key-append-only"> append-only</label>
        </div>
        <label class="field-label">Public key</label>
        <textarea id="key-pubkey" class="field-input mb-3" rows="2" placeholder="ssh-ed25519 AAAA... you@host" required></textarea>
        <button type="submit" class="btn btn-primary">${icon('fa-plus')}<span>Add key</span></button>
      </form>
    </div>`;

  // The repository picker in "Register a key" always tracks whichever
  // tenant is currently selected there (or the signed-in tenant, for a
  // non-admin session) — a repo belongs to exactly one tenant, so the
  // dropdown must be refreshed every time that selection changes rather
  // than showing a fixed, possibly-wrong list.
  async function refreshKeyRepoOptions() {
    const tenant = me.admin ? document.getElementById('key-tenant').value : me.username;
    const repoSelect = document.getElementById('key-repo');
    repoSelect.innerHTML = optionsHTML(await fetchRepoNames(tenant), 'no repositories yet — create one first');
  }

  async function loadKeys() {
    const tenant = me.admin ? document.getElementById('keys-filter-tenant').value : me.username;
    if (!tenant) {
      document.getElementById('keys-rows').innerHTML = `<tr><td colspan="4" class="text-slate-400 text-center py-4">No tenants yet.</td></tr>`;
      return;
    }
    const qs = '?tenant=' + encodeURIComponent(tenant);
    let keys = [];
    try { keys = (await api('GET', '/api/keys' + qs)) || []; } catch { keys = []; }
    const tbody = document.getElementById('keys-rows');
    if (keys.length === 0) {
      tbody.innerHTML = `<tr><td colspan="4" class="text-slate-400 text-center py-4">No keys registered yet.</td></tr>`;
      return;
    }
    tbody.innerHTML = keys.map((k) => `
      <tr>
        <td class="font-mono text-xs">${escapeHTML(k.RepoPath)}</td>
        <td>${k.AppendOnly ? `<span class="badge badge-warn">append-only</span>` : `<span class="badge badge-info">read-write</span>`}</td>
        <td class="text-slate-500">${escapeHTML(k.Comment || '(no comment)')}</td>
        <td class="text-right"><button class="rowbtn" data-material="${escapeHTML(k.KeyMaterial)}">remove</button></td>
      </tr>`).join('');
    tbody.querySelectorAll('button[data-material]').forEach((btn) => {
      btn.addEventListener('click', async () => {
        const res = await confirmModal({ title: 'Remove this key?', body: 'The matching private key will no longer be able to reach this repository.', confirmLabel: 'Remove', danger: true });
        if (!res) return;
        try {
          await api('DELETE', `/api/keys/${encodeURIComponent(tenant)}/${encodeURIComponent(btn.dataset.material)}`);
          toast('Key removed', 'ok');
          loadKeys();
        } catch (err) {
          toast('Remove failed: ' + err.message, 'err');
        }
      });
    });
  }

  if (me.admin) {
    document.getElementById('keys-filter-tenant').addEventListener('change', loadKeys);
    document.getElementById('key-tenant').addEventListener('change', refreshKeyRepoOptions);
  }
  document.getElementById('key-add-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const body = {
      repo: document.getElementById('key-repo').value,
      publicKey: document.getElementById('key-pubkey').value.trim(),
      appendOnly: document.getElementById('key-append-only').checked,
    };
    if (me.admin) body.tenant = document.getElementById('key-tenant').value;
    try {
      await api('POST', '/api/keys', body);
      document.getElementById('key-pubkey').value = '';
      toast('Key registered', 'ok');
      loadKeys();
    } catch (err) {
      toast('Could not add key: ' + err.message, 'err');
    }
  });

  await refreshKeyRepoOptions();
  await loadKeys();
};

// ---- Users (admin only) ----------------------------------------------------------

views.users = async (container) => {
  container.innerHTML = `
    <h2 class="page-title">${icon('fa-users', 'text-indigo-500')}Accounts</h2>
    <div class="card">
      <h3 class="card-title">${icon('fa-list')}All accounts</h3>
      <table class="data-table">
        <thead><tr><th>Username</th><th>Role</th><th>Keys</th><th></th></tr></thead>
        <tbody id="users-rows"></tbody>
      </table>
    </div>
    <div class="card">
      <h3 class="card-title">${icon('fa-user-plus')}Create account</h3>
      <form id="user-create-form" class="flex flex-wrap items-end gap-3">
        <div><label class="field-label">Username</label><input type="text" id="user-new-name" class="field-input" required></div>
        <label class="field-checkbox-row"><input type="checkbox" id="user-new-admin"> admin</label>
        <button type="submit" class="btn btn-primary">${icon('fa-plus')}<span>Create account</span></button>
      </form>
      <p class="text-xs text-slate-500 mt-3">A new account is created locked — use "set password" below once it appears in the list.</p>
    </div>`;

  async function loadUsers() {
    const users = (await api('GET', '/api/users')) || [];
    const tbody = document.getElementById('users-rows');
    tbody.innerHTML = users.map((u) => `
      <tr>
        <td class="font-medium text-slate-900">${escapeHTML(u.username)}${u.username === me.username ? ' <span class="text-slate-400 text-xs">(you)</span>' : ''}</td>
        <td>${u.admin ? `<span class="badge badge-info">admin</span>` : `<span class="badge badge-off">tenant</span>`}
          ${u.disabled ? `<span class="badge badge-warn ml-1">${icon('fa-ban', 'mr-0.5')}disabled</span>` : ''}</td>
        <td>${u.keys}</td>
        <td class="text-right space-x-3">
          <button class="rowbtn text-indigo-600" data-action="passwd" data-user="${escapeHTML(u.username)}">set password</button>
          <button class="rowbtn text-indigo-600" data-action="${u.disabled ? 'enable' : 'disable'}" data-user="${escapeHTML(u.username)}">${u.disabled ? 'enable' : 'disable'}</button>
          <button class="rowbtn" data-action="delete" data-user="${escapeHTML(u.username)}">delete</button>
        </td>
      </tr>`).join('');

    tbody.querySelectorAll('button[data-action="passwd"]').forEach((btn) => {
      btn.addEventListener('click', () => promptSetPassword(btn.dataset.user, btn.dataset.user === me.username));
    });
    tbody.querySelectorAll('button[data-action="disable"], button[data-action="enable"]').forEach((btn) => {
      btn.addEventListener('click', async () => {
        const username = btn.dataset.user, action = btn.dataset.action;
        if (action === 'disable' && username === me.username) {
          const res = await confirmModal({
            title: 'Disable your own account?',
            body: 'You will be signed out and locked out of the web UI and SSH immediately. Only another admin (or the CLI, as root) can re-enable it.',
            confirmLabel: 'Disable my account',
            danger: true,
          });
          if (!res) return;
        }
        try {
          await api('POST', `/api/users/${encodeURIComponent(username)}/${action}`);
          toast(action === 'disable' ? `${username} is disabled` : `${username} is enabled`, 'ok');
          if (action === 'disable' && username === me.username) { me = null; showLogin(); return; }
          loadUsers();
        } catch (err) {
          toast(`Could not ${action}: ` + err.message, 'err');
        }
      });
    });
    tbody.querySelectorAll('button[data-action="delete"]').forEach((btn) => {
      btn.addEventListener('click', async () => {
        const username = btn.dataset.user;
        const res = await confirmModal({
          title: `Delete ${username}?`,
          body: 'Their repositories are not touched by this — remove or purge those separately if you want the data gone too.',
          confirmLabel: 'Delete account',
          danger: true,
          requireText: username,
        });
        if (!res) return;
        try {
          await api('DELETE', `/api/users/${encodeURIComponent(username)}`);
          toast('Account deleted', 'ok');
          loadUsers();
        } catch (err) {
          toast('Delete failed: ' + err.message, 'err');
        }
      });
    });
  }

  document.getElementById('user-create-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    try {
      await api('POST', '/api/users', {
        username: document.getElementById('user-new-name').value.trim(),
        admin: document.getElementById('user-new-admin').checked,
      });
      document.getElementById('user-new-name').value = '';
      document.getElementById('user-new-admin').checked = false;
      toast('Account created — set a password next', 'ok');
      loadUsers();
    } catch (err) {
      toast('Could not create account: ' + err.message, 'err');
    }
  });

  await loadUsers();
};

// ---- Doctor (admin only) ----------------------------------------------------------

views.doctor = async (container) => {
  container.innerHTML = `
    <h2 class="page-title">${icon('fa-heartbeat', 'text-indigo-500')}Doctor</h2>
    <div class="card">
      <div class="flex items-center justify-between mb-3">
        <h3 class="card-title mb-0">${icon('fa-stethoscope')}Diagnostics</h3>
        <button id="doctor-run" class="btn btn-sm">${icon('fa-refresh')}<span>Run again</span></button>
      </div>
      <table class="data-table">
        <thead><tr><th>Check</th><th>Status</th><th>Detail</th></tr></thead>
        <tbody id="doctor-rows"><tr><td colspan="3" class="text-slate-400 text-center py-4">Running…</td></tr></tbody>
      </table>
    </div>`;

  async function run() {
    const checks = (await api('GET', '/api/doctor')) || [];
    document.getElementById('doctor-rows').innerHTML = checks.map((c) => `
      <tr>
        <td class="font-medium text-slate-900">${escapeHTML(c.Name)}</td>
        <td><span class="badge ${badgeClass(c.Status)}">${escapeHTML(c.Status)}</span></td>
        <td class="text-slate-600">${escapeHTML(c.Detail)}${c.Fix ? ` <span class="text-slate-400">— ${escapeHTML(c.Fix)}</span>` : ''}</td>
      </tr>`).join('');
  }
  document.getElementById('doctor-run').addEventListener('click', run);
  await run();
};

// ---- bootstrap ------------------------------------------------------------------

api('GET', '/api/me')
  .then((m) => { me = m; showApp(); })
  .catch(() => showLogin());
