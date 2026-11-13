let currentUser = null;
let authAppStarted = false;
let authExpiredHandling = false;

function hasPermission(name) {
  return Boolean(currentUser && (currentUser.is_admin || currentUser[name]));
}

function showAuthPanel(mode) {
  $('#appShell').classList.add('hidden');
  $('#authScreen').classList.remove('hidden');
  $('#authLoginPanel').classList.toggle('hidden', mode !== 'login');
  $('#authSetupPanel').classList.toggle('hidden', mode !== 'setup');
  $('#authError').textContent = '';
  if (mode === 'login') {
    $('#authLoginPassword').value = '';
    setTimeout(() => $('#authLoginUsername').focus(), 30);
  } else {
    setTimeout(() => $('#authSetupUsername').focus(), 30);
  }
}

function applyUserPermissions() {
  const isAdmin = Boolean(currentUser && currentUser.is_admin);
  $$('[data-admin-only]').forEach(element => {
    element.classList.toggle('hidden', !isAdmin);
  });
  $$('[data-permission]').forEach(element => {
    element.classList.toggle('hidden', !hasPermission(element.dataset.permission));
  });
  const canEnroll = hasPermission('can_add_students') || hasPermission('can_resample_faces');
  $('#enrollmentWorkbench')?.classList.toggle('hidden', !canEnroll);
  $('#photoImportCard')?.classList.toggle('hidden', !canEnroll);
  $('#sampleControls')?.classList.toggle('hidden', !hasPermission('can_resample_faces'));
  $('#startSupplement')?.classList.toggle('hidden', !hasPermission('can_resample_faces'));
  $('#duplicateUpdateProfile')?.classList.toggle('hidden', !hasPermission('can_edit_students'));
  $('#currentUserLabel').textContent = currentUser
    ? (currentUser.is_admin ? '管理员：' : '用户：') + currentUser.username
    : '';
}

function enterAuthenticatedApp() {
  if (!currentUser) return;
  authExpiredHandling = false;
  $('#authScreen').classList.add('hidden');
  $('#appShell').classList.remove('hidden');
  applyUserPermissions();
  if (!authAppStarted && typeof startAuthenticatedApp === 'function') {
    authAppStarted = true;
    startAuthenticatedApp();
  }
}

async function initializeAuth() {
  const status = await api('/api/auth/status');
  if (status.setup_required) {
    showAuthPanel('setup');
    return false;
  }
  if (!status.authenticated || !status.user) {
    showAuthPanel('login');
    return false;
  }
  currentUser = status.user;
  enterAuthenticatedApp();
  return true;
}

function handleAuthExpired() {
  if (authExpiredHandling) return;
  authExpiredHandling = true;
  currentUser = null;
  authAppStarted = false;
  if (typeof stopAutoRecognition === 'function') stopAutoRecognition();
  if (typeof stopCamera === 'function') stopCamera().catch(() => {});
  showAuthPanel('login');
  $('#authError').textContent = '登录已过期，请重新登录。';
}

$('#authLoginForm').addEventListener('submit', async event => {
  event.preventDefault();
  const button = $('#authLoginForm').querySelector('button[type="submit"]');
  button.disabled = true;
  $('#authError').textContent = '';
  try {
    const result = await api('/api/auth/login', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        username: $('#authLoginUsername').value.trim(),
        password: $('#authLoginPassword').value
      })
    });
    currentUser = result.user;
    enterAuthenticatedApp();
  } catch (error) {
    $('#authError').textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

$('#authSetupForm').addEventListener('submit', async event => {
  event.preventDefault();
  const password = $('#authSetupPassword').value;
  if (password !== $('#authSetupPasswordConfirm').value) {
    $('#authError').textContent = '两次输入的密码不一致。';
    return;
  }
  const button = $('#authSetupForm').querySelector('button[type="submit"]');
  button.disabled = true;
  $('#authError').textContent = '';
  try {
    const result = await api('/api/auth/setup', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        username: $('#authSetupUsername').value.trim(),
        password
      })
    });
    currentUser = result.user;
    enterAuthenticatedApp();
  } catch (error) {
    $('#authError').textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

$('#logoutButton').addEventListener('click', async () => {
  try {
    await api('/api/auth/logout', {method: 'POST'});
  } catch (error) {
    // The local session is cleared even if the server is unavailable.
  }
  currentUser = null;
  authAppStarted = false;
  if (typeof stopAutoRecognition === 'function') stopAutoRecognition();
  if (typeof stopCamera === 'function') await stopCamera().catch(() => {});
  showAuthPanel('login');
});

$('#changePasswordOpen').addEventListener('click', () => {
  $('#changePasswordForm').reset();
  openModal('changePasswordModal');
});

$('#changePasswordForm').addEventListener('submit', async event => {
  event.preventDefault();
  const next = $('#newPassword').value;
  if (next !== $('#newPasswordConfirm').value) {
    toast('两次输入的新密码不一致');
    return;
  }
  const button = $('#changePasswordForm').querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    await api('/api/auth/password', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        current_password: $('#currentPassword').value,
        new_password: next
      })
    });
    closeModal('changePasswordModal');
    toast('登录密码已更新');
  } catch (error) {
    toast(error.message);
  } finally {
    button.disabled = false;
  }
});
