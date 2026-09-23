let appSettingsCache = {auto_start_checkin: false};
let appSettingsLoaded = false;

function renderAppSettings() {
  const checkbox = $('#autoStartCheckin');
  const state = $('#autoStartCheckinState');
  const enabled = Boolean(appSettingsCache.auto_start_checkin);
  if (checkbox) checkbox.checked = enabled;
  if (state) state.textContent = enabled ? '已开启' : '已关闭';
}

async function loadAppSettings(force = false) {
  if (appSettingsLoaded && !force) {
    renderAppSettings();
    return appSettingsCache;
  }
  appSettingsCache = await api('/api/settings');
  appSettingsLoaded = true;
  renderAppSettings();
  return appSettingsCache;
}

async function enterCheckinPageAutoStart() {
  await loadAppSettings();
  if (!appSettingsCache.auto_start_checkin) return;
  try {
    await setAutoRecognitionEnabled(true);
  } catch {
    // Camera startup already reports its own HTTPS/device error.
  }
}

$('#autoStartCheckin').addEventListener('change', async event => {
  const enabled = event.target.checked;
  event.target.disabled = true;
  try {
    appSettingsCache = await api('/api/settings', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({auto_start_checkin: enabled})
    });
    appSettingsLoaded = true;
    renderAppSettings();
    toast(enabled ? '已开启：进入人脸签到时自动开始签到' : '已关闭：进入人脸签到时手动开始签到');
  } catch (e) {
    event.target.checked = !enabled;
    toast(e.message);
  } finally {
    event.target.disabled = false;
  }
});
