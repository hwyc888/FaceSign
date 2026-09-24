const REALTIME_STATUS_FIELDS = [
  ['realtimeStatusMode', 'realtime_status_mode', 'mode'],
  ['realtimeStatusVideo', 'realtime_status_video', 'video'],
  ['realtimeStatusDrop', 'realtime_status_drop', 'drop'],
  ['realtimeStatusNetwork', 'realtime_status_network', 'network'],
  ['realtimeStatusRecognition', 'realtime_status_recognition', 'recognition'],
  ['realtimeStatusReason', 'realtime_status_reason', 'reason']
];

let appSettingsCache = {
  auto_start_checkin: false,
  realtime_status_enabled: true,
  realtime_status_mode: true,
  realtime_status_video: true,
  realtime_status_drop: true,
  realtime_status_network: true,
  realtime_status_recognition: true,
  realtime_status_reason: true
};
let appSettingsLoaded = false;

function renderAppSettings() {
  const checkbox = $('#autoStartCheckin');
  const state = $('#autoStartCheckinState');
  const enabled = Boolean(appSettingsCache.auto_start_checkin);
  if (checkbox) checkbox.checked = enabled;
  if (state) state.textContent = enabled ? '已开启' : '已关闭';

  const realtimeCheckbox = $('#realtimeStatusEnabled');
  const realtimeState = $('#realtimeStatusEnabledState');
  const realtimeEnabled = appSettingsCache.realtime_status_enabled !== false;
  if (realtimeCheckbox) realtimeCheckbox.checked = realtimeEnabled;
  if (realtimeState) realtimeState.textContent = realtimeEnabled ? '已显示' : '已隐藏';

  const realtimeFields = {};
  REALTIME_STATUS_FIELDS.forEach(([id, key, field]) => {
    const visible = appSettingsCache[key] !== false;
    const fieldCheckbox = $('#' + id);
    if (fieldCheckbox) fieldCheckbox.checked = visible;
    realtimeFields[field] = visible;
  });

  if (typeof setCameraRealtimeStatusEnabled === 'function') {
    setCameraRealtimeStatusEnabled(realtimeEnabled);
  }
  if (typeof setCameraRealtimeStatusFields === 'function') {
    setCameraRealtimeStatusFields(realtimeFields);
  }
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
  try {
    if (appSettingsCache.auto_start_checkin) {
      await setAutoRecognitionEnabled(true);
      return;
    }
    if (cameraOpen) {
      await startCamera();
    }
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

$('#realtimeStatusEnabled').addEventListener('change', async event => {
  const enabled = event.target.checked;
  event.target.disabled = true;
  try {
    appSettingsCache = await api('/api/settings', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({realtime_status_enabled: enabled})
    });
    appSettingsLoaded = true;
    renderAppSettings();
    toast(enabled ? '已显示摄像头实时状态' : '已隐藏摄像头实时状态');
  } catch (e) {
    event.target.checked = !enabled;
    toast(e.message);
  } finally {
    event.target.disabled = false;
  }
});

REALTIME_STATUS_FIELDS.forEach(([id, key]) => {
  const checkbox = $('#' + id);
  if (!checkbox) return;
  checkbox.addEventListener('change', async event => {
    const enabled = event.target.checked;
    event.target.disabled = true;
    try {
      appSettingsCache = await api('/api/settings', {
        method: 'PUT',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({[key]: enabled})
      });
      appSettingsLoaded = true;
      renderAppSettings();
    } catch (e) {
      event.target.checked = !enabled;
      toast(e.message);
    } finally {
      event.target.disabled = false;
    }
  });
});

const clearRecognitionStatsButton = $('#clearRecognitionStats');
if (clearRecognitionStatsButton) {
  clearRecognitionStatsButton.addEventListener('click', async () => {
    if (!window.confirm('确认清零“已验证”和“未录入”的累计统计吗？\n不会删除考勤记录、学生资料或人脸样本。')) return;
    clearRecognitionStatsButton.disabled = true;
    try {
      const stats = await api('/api/recognition-stats', {method: 'DELETE'});
      if (typeof renderRecognitionStats === 'function') renderRecognitionStats(stats);
      toast('识别累计统计已清零');
    } catch (e) {
      toast(e.message);
    } finally {
      clearRecognitionStatsButton.disabled = false;
    }
  });
}

