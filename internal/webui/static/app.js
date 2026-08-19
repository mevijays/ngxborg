// ngxborg web UI — plain JS, no build step, matching the rest of this
// project's preference for dependency-free tooling.
'use strict';

let me = null; // {username, admin}

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401) {
    showLogin();
    throw new Error('session expired');
  }
  let data = null;
  const text = await res.text();
  if (text) data = JSON.parse(text);
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

function el(id) { return document.getElementById(id); }

function showLogin() {
  el('login-view').hidden = false;
  el('app-view').hidden = true;
}

function showApp() {
  el('login-view').hidden = true;
  el('app-view').hidden = false;
  el('whoami').textContent = me.username + (me.admin ? ' (admin)' : '');
  document.querySelectorAll('.admin-only').forEach((n) => (n.hidden = !me.admin));
}

el('login-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  el('login-error').textContent = '';
  try {
    me = await api('POST', '/api/login', {
      username: el('login-username').value,
      password: el('login-password').value,
    });
    el('login-password').value = '';
    showApp();
    loadRepos();
  } catch (err) {
    el('login-error').textContent = err.message;
  }
});

el('logout-btn').addEventListener('click', async () => {
  await api('POST', '/api/logout');
  me = null;
  showLogin();
});

document.querySelectorAll('.tab').forEach((btn) => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.tab').forEach((b) => b.classList.remove('active'));
    document.querySelectorAll('.tab-panel').forEach((p) => (p.hidden = true));
    btn.classList.add('active');
    const panel = el('tab-' + btn.dataset.tab);
    panel.hidden = false;
    if (btn.dataset.tab === 'repos') loadRepos();
    if (btn.dataset.tab === 'keys') loadKeys();
    if (btn.dataset.tab === 'users') loadUsers();
    if (btn.dataset.tab === 'doctor') loadDoctor();
  });
});

// ---- repos ------------------------------------------------------------

async function loadRepos() {
  if (me && me.admin) el('repo-tenant-filter').hidden = false;
  const tenant = me && me.admin ? el('repo-tenant').value.trim() : '';
  const qs = tenant ? '?tenant=' + encodeURIComponent(tenant) : '';
  const repos = await api('GET', '/api/repos' + qs);
  const tbody = document.querySelector('#repo-table tbody');
  tbody.innerHTML = '';
  (repos || []).forEach((r) => {
    const tr = document.createElement('tr');
    const status = r.Initialized ? (r.SizeMB || 0) + ' MB' : 'empty (not yet initialized)';
    tr.innerHTML =
      '<td>' + esc(r.Tenant) + '</td><td>' + esc(r.Name) + '</td><td>' + esc(status) +
      '</td><td>' + esc((r.CreatedAt || '').slice(0, 10)) + '</td><td></td>';
    const del = document.createElement('button');
    del.className = 'rowbtn';
    del.textContent = 'delete';
    del.onclick = async () => {
      if (!confirm('Move ' + r.Tenant + '/' + r.Name + ' to trash?')) return;
      await api('DELETE', '/api/repos/' + enc(r.Tenant) + '/' + enc(r.Name));
      loadRepos();
    };
    tr.lastElementChild.appendChild(del);
    tbody.appendChild(tr);
  });
}

el('repo-tenant-go').addEventListener('click', loadRepos);

el('repo-create-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const tenant = me.admin ? el('repo-new-tenant').value.trim() : undefined;
  const name = el('repo-new-name').value.trim();
  try {
    const repo = await api('POST', '/api/repos', { tenant, name });
    el('repo-new-name').value = '';
    el('repo-create-hint').textContent =
      'Register a key for it, then from the tenant\'s own machine: borg init --encryption=repokey-blake2 ssh://' +
      repo.Tenant + '@<this-host>:<borg-port>' + repo.Path;
    loadRepos();
  } catch (err) {
    el('repo-create-hint').textContent = err.message;
  }
});

if (me && me.admin) el('repo-new-tenant').hidden = false;

// ---- keys ---------------------------------------------------------------

async function loadKeys() {
  if (me && me.admin) el('keys-tenant').hidden = false;
  const tenant = me && me.admin ? el('keys-tenant').value.trim() : '';
  const qs = tenant ? '?tenant=' + encodeURIComponent(tenant) : '';
  const keys = await api('GET', '/api/keys' + qs);
  const tbody = document.querySelector('#keys-table tbody');
  tbody.innerHTML = '';
  (keys || []).forEach((k) => {
    const tr = document.createElement('tr');
    const mode = k.AppendOnly ? 'append-only' : 'read-write';
    tr.innerHTML =
      '<td>' + esc(k.RepoPath) + '</td><td>' + mode + '</td><td>' + esc(k.Comment || '') + '</td><td></td>';
    const del = document.createElement('button');
    del.className = 'rowbtn';
    del.textContent = 'remove';
    del.onclick = async () => {
      const t = me.admin ? el('keys-tenant').value.trim() : me.username;
      await api('DELETE', '/api/keys/' + enc(t) + '/' + enc(k.KeyMaterial));
      loadKeys();
    };
    tr.lastElementChild.appendChild(del);
    tbody.appendChild(tr);
  });
}

el('key-add-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const tenant = me.admin ? el('keys-tenant').value.trim() : undefined;
  try {
    await api('POST', '/api/keys', {
      tenant,
      repo: el('key-repo').value.trim(),
      publicKey: el('key-pubkey').value.trim(),
      appendOnly: el('key-append-only').checked,
    });
    el('key-repo').value = '';
    el('key-pubkey').value = '';
    loadKeys();
  } catch (err) {
    alert(err.message);
  }
});

// ---- users (admin) --------------------------------------------------------

async function loadUsers() {
  const users = await api('GET', '/api/users');
  const tbody = document.querySelector('#users-table tbody');
  tbody.innerHTML = '';
  (users || []).forEach((u) => {
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(u.username) + '</td><td>' + (u.admin ? 'admin' : 'tenant') + '</td><td>' + u.keys + '</td><td></td>';
    tbody.appendChild(tr);
  });
}

el('user-create-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  try {
    await api('POST', '/api/users', {
      username: el('user-new-name').value.trim(),
      admin: el('user-new-admin').checked,
    });
    el('user-new-name').value = '';
    loadUsers();
  } catch (err) {
    alert(err.message);
  }
});

// ---- doctor (admin) -------------------------------------------------------

async function loadDoctor() {
  const checks = await api('GET', '/api/doctor');
  const tbody = document.querySelector('#doctor-table tbody');
  tbody.innerHTML = '';
  (checks || []).forEach((c) => {
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(c.Name) + '</td><td class="status-' + c.Status + '">' + c.Status +
      '</td><td>' + esc(c.Detail) + (c.Fix ? ' — ' + esc(c.Fix) : '') + '</td>';
    tbody.appendChild(tr);
  });
}
el('doctor-run').addEventListener('click', loadDoctor);

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function enc(s) { return encodeURIComponent(s); }

// ---- bootstrap ------------------------------------------------------------

api('GET', '/api/me')
  .then((m) => {
    me = m;
    showApp();
    loadRepos();
  })
  .catch(() => showLogin());
