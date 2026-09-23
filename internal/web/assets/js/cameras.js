let editingCameraID = 0;
let localCameraDevices = [];
let cameraTestPreviewURL = null;

const CAMERA_PRESETS = {
  hikvision: {
    label: '海康 Hikvision',
    username: 'admin',
    stream: ip => `rtsp://${ip}:554/Streaming/channels/101`,
    snapshot: ip => `http://${ip}/ISAPI/Streaming/channels/1/picture`
  },
  dahua: {
    label: '大华 Dahua',
    username: 'admin',
    stream: ip => `rtsp://${ip}:554/cam/realmonitor?channel=1&subtype=0`,
    snapshot: ip => `http://${ip}/cgi-bin/snapshot.cgi?channel=1`
  },
  uniview: {
    label: '宇视 Uniview',
    username: 'admin',
    stream: ip => `rtsp://${ip}:554/media/video1`,
    snapshot: ip => `http://${ip}/LAPI/V1.0/Channels/1/Media/Video/Streams/0/Snapshot`
  },
  vivotek: {
    label: 'VIVOTEK / 晶睿',
    username: 'root',
    stream: ip => `rtsp://${ip}:554/live.sdp`,
    snapshot: ip => `http://${ip}/cgi-bin/viewer/video.jpg?streamid=0`
  },
  axis: {
    label: 'AXIS',
    username: 'root',
    stream: ip => `rtsp://${ip}:554/axis-media/media.amp`,
    snapshot: ip => `http://${ip}/axis-cgi/jpg/image.cgi?camera=1`
  }
};

let cameraAdvancedOpen = false;

function normalizeCameraIP(value) {
  const ip = String(value || '').trim();
  if (!ip) return '';
  if (ip.includes('://') || /[\\/?#:\s]/.test(ip)) {
    throw new Error('IP 地址只填写数字和点，例如 192.168.1.64；不要带 http://、端口或路径');
  }
  const parts = ip.split('.');
  if (parts.length !== 4 || parts.some(part => !/^\d{1,3}$/.test(part) || Number(part) > 255)) {
    throw new Error('请输入正确的 IPv4 地址，例如 192.168.1.64');
  }
  return parts.map(part => String(Number(part))).join('.');
}

function cameraPresetFromCamera(camera) {
  const stream = String(camera?.stream_url || '').toLowerCase();
  const snapshot = String(camera?.snapshot_url || '').toLowerCase();
  if (snapshot.includes('/isapi/streaming/channels/') || stream.includes('/streaming/channels/')) return 'hikvision';
  if (snapshot.includes('/cgi-bin/snapshot.cgi') || stream.includes('/cam/realmonitor')) return 'dahua';
  if (snapshot.includes('/lapi/v1.0/channels/') || stream.includes('/media/video1')) return 'uniview';
  if (snapshot.includes('/cgi-bin/viewer/video.jpg') || stream.includes('/live.sdp')) return 'vivotek';
  if (snapshot.includes('/axis-cgi/jpg/image.cgi') || stream.includes('/axis-media/media.amp')) return 'axis';
  return 'custom';
}

function cameraIPFromCamera(camera) {
  for (const raw of [camera?.snapshot_url, camera?.stream_url]) {
    if (!raw) continue;
    try {
      const host = new URL(raw).hostname;
      if (host) return host;
    } catch {}
  }
  return '';
}

function applyCameraPreset({requireIP = false} = {}) {
  if ($('#cameraKind')?.value !== 'network') return;
  const presetID = $('#cameraPreset')?.value || 'hikvision';
  const preset = CAMERA_PRESETS[presetID];
  if (!preset) {
    $('#cameraPresetSummary').textContent = '自定义模式：请在高级参数中填写设备实际地址。';
    return;
  }

  let ip = '';
  try {
    ip = normalizeCameraIP($('#cameraIP').value);
  } catch (e) {
    $('#cameraPresetSummary').textContent = e.message;
    if (requireIP) throw e;
    return;
  }
  if (!ip) {
    $('#cameraStreamURL').value = '';
    $('#cameraSnapshotURL').value = '';
    $('#cameraPresetSummary').textContent = `已选择 ${preset.label}；现在只需填写摄像头 IP、用户名和密码。`;
    if (requireIP) throw new Error('请填写摄像头 IP 地址');
    return;
  }

  $('#cameraProtocol').value = 'rtsp';
  $('#cameraStreamURL').value = preset.stream(ip);
  $('#cameraSnapshotURL').value = preset.snapshot(ip);
  $('#cameraAuthMode').value = 'auto';
  if (!$('#cameraUsername').value.trim()) {
    $('#cameraUsername').placeholder = `常用用户名：${preset.username}`;
  }
  $('#cameraPresetTitle').textContent = `${preset.label} · 已自动配置`;
  $('#cameraPresetSummary').textContent = `IP：${ip}；RTSP、抓图路径和认证方式已由 FaceSign 自动生成，无需手工填写。`;
}

function cameraNetworkDisplay(camera) {
  const presetID = cameraPresetFromCamera(camera);
  const ip = cameraIPFromCamera(camera);
  const preset = CAMERA_PRESETS[presetID];
  if (ip && preset) return `${ip} · ${preset.label}`;
  if (ip) return ip;
  return camera.snapshot_url || camera.stream_url || '-';
}


function cameraFormPayload() {
  const kind = $('#cameraKind').value;
  if (kind === 'network' && $('#cameraPreset').value !== 'custom') {
    applyCameraPreset({requireIP: true});
  }
  const payload = {
    camera_id: editingCameraID || 0,
    name: $('#cameraName').value.trim() || '连接测试',
    kind,
    device_id: kind === 'local' ? $('#cameraDevice').value : '',
    protocol: kind === 'local' ? 'browser' : $('#cameraProtocol').value,
    stream_url: kind === 'network' ? $('#cameraStreamURL').value.trim() : '',
    snapshot_url: kind === 'network' ? $('#cameraSnapshotURL').value.trim() : '',
    username: kind === 'network' ? $('#cameraUsername').value.trim() : '',
    auth_mode: kind === 'network' ? $('#cameraAuthMode').value : 'none',
    agent_id: kind === 'agent' ? $('#cameraAgentID').value.trim() : '',
    width: Number($('#cameraWidth').value),
    height: Number($('#cameraHeight').value),
    fps: Number($('#cameraFPS').value),
    timeout_ms: Number($('#cameraTimeout').value),
    tls_insecure: kind === 'network' && $('#cameraTLSInsecure').checked,
    is_default: $('#cameraDefault').checked,
    clear_password: editingCameraID > 0 && $('#cameraClearPassword').checked
  };
  const password = $('#cameraPassword').value;
  if (kind === 'network' && password) payload.password = password;
  const agentSecret = $('#cameraAgentSecret').value.trim();
  if (kind === 'agent' && agentSecret) payload.agent_secret = agentSecret;
  return payload;
}

function resetCameraTestPanel(message = '填写摄像头参数后点击“测试连接”，系统会检查参数、网络、认证和图像抓取。') {
  const headline = $('#cameraTestHeadline');
  if (headline) {
    headline.textContent = '尚未测试连接';
    headline.className = 'camera-test-headline neutral';
  }
  const result = $('#cameraTestResult');
  if (result) result.textContent = message;
  const checks = $('#cameraTestChecks');
  if (checks) checks.innerHTML = '';
  if (cameraTestPreviewURL) {
    URL.revokeObjectURL(cameraTestPreviewURL);
    cameraTestPreviewURL = null;
  }
  const preview = $('#cameraTestPreview');
  if (preview) {
    preview.removeAttribute('src');
    preview.classList.add('hidden');
  }
}

function renderCameraConnectionTest(result) {
  const headline = $('#cameraTestHeadline');
  const summary = $('#cameraTestResult');
  const checks = $('#cameraTestChecks');
  if (headline) {
    headline.textContent = result.ok ? '连接成功' : '连接失败';
    headline.className = 'camera-test-headline ' + (result.ok ? 'success' : 'error');
  }
  if (summary) {
    const authSuffix = result.detected_auth
      ? ` · 自动检测认证：${result.detected_auth === 'none' ? '无需认证' : result.detected_auth.toUpperCase()}`
      : '';
    const suffix = result.elapsed_ms > 0 ? ` · ${result.elapsed_ms} ms` : '';
    summary.textContent = (result.message || (result.ok ? '连接成功' : '连接失败')) + authSuffix + suffix;
  }
  if (checks) {
    checks.innerHTML = (result.checks || []).map(check => `
      <div class="camera-test-check ${esc(check.status || 'pending')}">
        <span class="camera-test-check-icon">${check.status === 'ok' ? '✓' : check.status === 'error' ? '×' : '…'}</span>
        <strong>${esc(check.name || '')}</strong>
        <span>${esc(check.message || '')}</span>
      </div>
    `).join('');
  }
  const preview = $('#cameraTestPreview');
  if (result.ok && result.preview_base64 && preview) {
    if (cameraTestPreviewURL) {
      URL.revokeObjectURL(cameraTestPreviewURL);
      cameraTestPreviewURL = null;
    }
    preview.src = 'data:image/jpeg;base64,' + result.preview_base64;
    preview.classList.remove('hidden');
  } else if (preview) {
    preview.removeAttribute('src');
    preview.classList.add('hidden');
  }
}

async function testCurrentCameraConfig() {
  const button = $('#testCameraConfig');
  const kind = $('#cameraKind').value;
  if (button) button.disabled = true;
  const headline = $('#cameraTestHeadline');
  if (headline) {
    headline.textContent = '正在测试...';
    headline.className = 'camera-test-headline testing';
  }
  $('#cameraTestResult').textContent = '正在检测摄像头连接，请稍候...';
  $('#cameraTestChecks').innerHTML = '';

  try {
    if (kind === 'local') {
      if (!window.isSecureContext) {
        throw new Error('本机摄像头测试需要 HTTPS 安全连接');
      }
      const camera = {
        width: Number($('#cameraWidth').value || 1280),
        height: Number($('#cameraHeight').value || 720),
        fps: Number($('#cameraFPS').value || 30),
        device_id: $('#cameraDevice').value
      };
      const started = performance.now();
      const testStream = await navigator.mediaDevices.getUserMedia(localVideoConstraints(camera));
      const settings = testStream.getVideoTracks()[0]?.getSettings?.() || {};
      testStream.getTracks().forEach(track => track.stop());
      renderCameraConnectionTest({
        ok: true,
        message: `本机摄像头可用：${settings.width || camera.width}×${settings.height || camera.height}，${Math.round(settings.frameRate || camera.fps)} FPS`,
        elapsed_ms: Math.round(performance.now() - started),
        checks: [
          {name: '浏览器权限', status: 'ok', message: '摄像头权限正常'},
          {name: '设备连接', status: 'ok', message: '视频设备可以打开'}
        ]
      });
      toast('摄像头测试成功');
      return;
    }

    if (kind === 'agent') {
      if (!editingCameraID) {
        renderCameraConnectionTest({
          ok: false,
          message: 'Camera Agent 需要先保存配置并让客户端连接后才能测试',
          checks: [
            {name: '配置状态', status: 'error', message: '请先保存 Camera Agent 配置'}
          ]
        });
        return;
      }
      await testCamera(editingCameraID);
      return;
    }

    const result = await api('/api/cameras/test', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(cameraFormPayload())
    });
    renderCameraConnectionTest(result);
    toast(result.ok ? '网络摄像头连接测试成功' : '连接测试失败：' + result.message);
  } catch (e) {
    renderCameraConnectionTest({
      ok: false,
      message: e.message,
      checks: [{name: '连接测试', status: 'error', message: e.message}]
    });
    toast('摄像头测试失败：' + e.message);
  } finally {
    if (button) button.disabled = false;
  }
}

function cameraTypeLabel(camera) {
  if (camera.kind === 'local') return '本机 / USB';
  if (camera.kind === 'agent') return '客户端 Camera Agent';
  if (camera.protocol === 'http_snapshot') return '网络 / HTTP抓图';
  if (camera.protocol === 'mjpeg') return '网络 / MJPEG';
  if (camera.protocol === 'rtsp') return '网络 / RTSP + HTTP抓图';
  return '网络摄像头';
}

function cameraSourceLabel(camera) {
  if (camera.kind === 'local') {
    if (!camera.device_id) return '浏览器系统默认设备';
    const found = localCameraDevices.find(device => device.deviceId === camera.device_id);
    return found?.label || ('设备 ID ' + camera.device_id.slice(0, 12) + '…');
  }
  if (camera.kind === 'agent') return camera.agent_id || '-';
  return cameraNetworkDisplay(camera);
}

function renderCameraRows() {
  const body = $('#camerasBody');
  if (!body) return;
  if (!camerasCache.length) {
    body.innerHTML = '<tr><td colspan="6" class="muted">尚未添加摄像头；当前仍可使用浏览器默认摄像头。</td></tr>';
    return;
  }
  body.innerHTML = camerasCache.map(camera => `
    <tr>
      <td><strong>${esc(camera.name)}</strong>${camera.is_default ? '<span class="camera-default-badge">默认</span>' : ''}</td>
      <td>${esc(cameraTypeLabel(camera))}</td>
      <td class="camera-source-cell" title="${esc(cameraSourceLabel(camera))}">${esc(cameraSourceLabel(camera))}</td>
      <td>${camera.width}×${camera.height} / ${camera.fps} FPS</td>
      <td>${camera.kind === 'network'
        ? esc(camera.auth_mode === 'auto' ? '自动检测' : camera.auth_mode === 'digest' ? 'Digest' : camera.auth_mode === 'basic' ? 'Basic' : '无认证')
        : camera.kind === 'agent'
          ? (camera.agent_online ? '<span class="camera-agent-online">在线</span>' : '<span class="camera-agent-offline">离线</span>')
          : '-'}</td>
      <td>
        <div class="camera-row-actions">
          ${camera.is_default ? '' : `<button data-camera-default="${camera.id}">设为默认</button>`}
          <button data-camera-test="${camera.id}">测试</button>
          <button data-camera-edit="${camera.id}">编辑</button>
          <button class="danger" data-camera-delete="${camera.id}">删除</button>
        </div>
      </td>
    </tr>
  `).join('');

  $$('[data-camera-default]').forEach(button => button.onclick = () => setDefaultCamera(Number(button.dataset.cameraDefault)));
  $$('[data-camera-test]').forEach(button => button.onclick = () => testCamera(Number(button.dataset.cameraTest)));
  $$('[data-camera-edit]').forEach(button => button.onclick = () => editCamera(Number(button.dataset.cameraEdit)));
  $$('[data-camera-delete]').forEach(button => button.onclick = () => deleteCamera(Number(button.dataset.cameraDelete)));
}

async function loadCameras() {
  await loadCameraConfigs(true);
  renderCameraRows();
  return camerasCache;
}

async function refreshLocalCameraDevices(requestPermission = false) {
  if (!navigator.mediaDevices?.enumerateDevices) {
    throw new Error('当前浏览器不支持摄像头设备枚举');
  }
  let temporary = null;
  if (requestPermission && !cameraOpen) {
    try {
      temporary = await navigator.mediaDevices.getUserMedia({video: true, audio: false});
    } catch (e) {
      throw new Error('无法读取本机摄像头列表：' + e.message);
    }
  }
  try {
    const devices = await navigator.mediaDevices.enumerateDevices();
    localCameraDevices = devices.filter(device => device.kind === 'videoinput');
    renderLocalCameraDeviceOptions();
    renderCameraRows();
    if (requestPermission) toast(`检测到 ${localCameraDevices.length} 个本机摄像头`);
  } finally {
    if (temporary) temporary.getTracks().forEach(track => track.stop());
  }
}

function renderLocalCameraDeviceOptions(selected = $('#cameraDevice')?.value || '') {
  const select = $('#cameraDevice');
  if (!select) return;
  const saved = selected;
  select.innerHTML = '<option value="">系统默认摄像头</option>' + localCameraDevices.map((device, index) =>
    `<option value="${esc(device.deviceId)}">${esc(device.label || ('摄像头 ' + (index + 1)))}</option>`
  ).join('');
  if (saved && !localCameraDevices.some(device => device.deviceId === saved)) {
    select.insertAdjacentHTML('beforeend', `<option value="${esc(saved)}">已保存的设备（当前未检测到）</option>`);
  }
  select.value = saved;
}

function updateCameraFormVisibility() {
  const kind = $('#cameraKind').value;
  const local = kind === 'local';
  const network = kind === 'network';
  const agent = kind === 'agent';
  const custom = network && $('#cameraPreset')?.value === 'custom';
  const showAdvanced = network && (custom || cameraAdvancedOpen);

  $('[data-camera-local]').forEach(el => el.classList.toggle('hidden', !local));
  $('[data-camera-network]').forEach(el => el.classList.toggle('hidden', !network));
  $('[data-camera-agent]').forEach(el => el.classList.toggle('hidden', !agent));
  $('[data-camera-advanced]').forEach(el => el.classList.toggle('hidden', !showAdvanced));

  const protocol = $('#cameraProtocol').value;
  $('#cameraStreamGroup').classList.toggle('hidden', !showAdvanced || protocol === 'http_snapshot');
  $('#cameraSnapshotGroup').classList.toggle('hidden', !showAdvanced);
  $('#cameraAdvancedToggle').textContent = showAdvanced && !custom ? '隐藏高级参数' : '查看高级参数';
  $('#cameraAdvancedToggle').classList.toggle('hidden', custom);

  $('#cameraSnapshotHelp').textContent = protocol === 'rtsp'
    ? '普通用户无需修改。RTSP用于保存视频流参数，人脸识别使用同一设备的HTTP/HTTPS抓图地址。'
    : protocol === 'mjpeg'
      ? '普通用户无需修改。MJPEG可直接取帧；如另有JPEG抓图地址会优先使用抓图地址。'
      : '普通用户无需修改。这里必须是直接返回JPEG/PNG图片的地址。';

  if (network && !custom) applyCameraPreset();
}

function resetCameraForm() {
  editingCameraID = 0;
  $('#cameraForm').reset();
  $('#cameraEditID').value = '';
  $('#cameraKind').value = 'local';
  $('#cameraPreset').value = 'hikvision';
  $('#cameraIP').value = '';
  cameraAdvancedOpen = false;
  $('#cameraProtocol').value = 'rtsp';
  $('#cameraAuthMode').value = 'auto';
  $('#cameraWidth').value = '1280';
  $('#cameraHeight').value = '720';
  $('#cameraFPS').value = '30';
  $('#cameraTimeout').value = '3000';
  $('#cameraFormTitle').textContent = '添加摄像头';
  $('#saveCamera').textContent = '添加摄像头';
  $('#cancelCameraEdit').classList.add('hidden');
  $('#cameraClearPasswordWrap').classList.add('hidden');
  $('#cameraPassword').placeholder = '网络摄像头密码';
  $('#cameraAgentID').value = '';
  $('#cameraAgentSecret').value = '';
  $('#cameraAgentSecret').placeholder = '建议使用随机生成密钥';
  renderLocalCameraDeviceOptions('');
  updateCameraFormVisibility();
  resetCameraTestPanel();
}

function editCamera(id) {
  const camera = camerasCache.find(item => item.id === id);
  if (!camera) return;
  editingCameraID = id;
  $('#cameraEditID').value = String(id);
  $('#cameraName').value = camera.name;
  $('#cameraKind').value = camera.kind;
  renderLocalCameraDeviceOptions(camera.device_id || '');
  const presetID = camera.kind === 'network' ? cameraPresetFromCamera(camera) : 'hikvision';
  $('#cameraPreset').value = presetID;
  $('#cameraIP').value = camera.kind === 'network' ? cameraIPFromCamera(camera) : '';
  cameraAdvancedOpen = presetID === 'custom';
  $('#cameraProtocol').value = camera.protocol === 'browser' ? 'http_snapshot' : camera.protocol;
  $('#cameraStreamURL').value = camera.stream_url || '';
  $('#cameraSnapshotURL').value = camera.snapshot_url || '';
  $('#cameraUsername').value = camera.username || '';
  $('#cameraPassword').value = '';
  $('#cameraPassword').placeholder = camera.has_password ? '已保存密码，留空不修改' : '网络摄像头密码';
  $('#cameraClearPassword').checked = false;
  $('#cameraClearPasswordWrap').classList.toggle('hidden', !camera.has_password);
  $('#cameraAuthMode').value = camera.auth_mode || 'auto';
  $('#cameraAgentID').value = camera.agent_id || '';
  $('#cameraAgentSecret').value = '';
  $('#cameraAgentSecret').placeholder = camera.has_agent_secret ? '已保存连接密钥；留空不修改' : '建议使用随机生成密钥';
  $('#cameraWidth').value = camera.width || 1280;
  $('#cameraHeight').value = camera.height || 720;
  $('#cameraFPS').value = camera.fps || (camera.kind === 'network' ? 5 : 30);
  $('#cameraTimeout').value = camera.timeout_ms || 3000;
  $('#cameraTLSInsecure').checked = !!camera.tls_insecure;
  $('#cameraDefault').checked = !!camera.is_default;
  $('#cameraFormTitle').textContent = '编辑摄像头';
  $('#saveCamera').textContent = '保存摄像头';
  $('#cancelCameraEdit').classList.remove('hidden');
  updateCameraFormVisibility();
  resetCameraTestPanel('已载入保存的摄像头参数，可直接修改后测试连接。');
  $('#cameraForm').scrollIntoView({behavior: 'smooth', block: 'start'});
}

async function setDefaultCamera(id) {
  try {
    await api(`/api/cameras/${id}/default`, {method: 'POST'});
    if (cameraOpen) stopCamera();
    await loadCameras();
    toast('默认摄像头已切换');
  } catch (e) {
    toast(e.message);
  }
}

async function deleteCamera(id) {
  const camera = camerasCache.find(item => item.id === id);
  if (!camera || !confirm(`确定删除摄像头“${camera.name}”吗？`)) return;
  try {
    if (activeCamera?.id === id && cameraOpen) stopCamera();
    await api(`/api/cameras/${id}`, {method: 'DELETE'});
    await loadCameras();
    if (editingCameraID === id) resetCameraForm();
    toast('摄像头已删除');
  } catch (e) {
    toast(e.message);
  }
}

async function testCamera(id) {
  const camera = camerasCache.find(item => item.id === id);
  if (!camera) return;
  try {
    const headline = $('#cameraTestHeadline');
    if (headline) {
      headline.textContent = '正在测试...';
      headline.className = 'camera-test-headline testing';
    }
    $('#cameraTestChecks').innerHTML = '';
    if (camera.kind === 'local') {
      const testStream = await navigator.mediaDevices.getUserMedia(localVideoConstraints(camera));
      const settings = testStream.getVideoTracks()[0]?.getSettings?.() || {};
      testStream.getTracks().forEach(track => track.stop());
      renderCameraConnectionTest({
        ok: true,
        message: `本机摄像头连接成功：${settings.width || camera.width}×${settings.height || camera.height}，${Math.round(settings.frameRate || camera.fps)} FPS`,
        checks: [
          {name: '浏览器权限', status: 'ok', message: '摄像头权限正常'},
          {name: '设备连接', status: 'ok', message: '视频设备可以打开'}
        ]
      });
    } else {
      const blob = await fetchCameraFrameBlob(camera.id);
      if (cameraTestPreviewURL) URL.revokeObjectURL(cameraTestPreviewURL);
      cameraTestPreviewURL = URL.createObjectURL(blob);
      $('#cameraTestPreview').src = cameraTestPreviewURL;
      $('#cameraTestPreview').classList.remove('hidden');
      renderCameraConnectionTest({
        ok: true,
        message: camera.kind === 'agent'
          ? `客户端 Agent“${camera.agent_id}”在线，画面接收成功。`
          : `网络摄像头“${camera.name}”连接及抓图成功。`,
        checks: [
          {name: '网络连接', status: 'ok', message: '摄像头/Agent 可以访问'},
          {name: '图像抓取', status: 'ok', message: '成功读取实时画面'}
        ]
      });
      $('#cameraTestPreview').src = cameraTestPreviewURL;
      $('#cameraTestPreview').classList.remove('hidden');
    }
    toast('摄像头测试成功');
  } catch (e) {
    renderCameraConnectionTest({
      ok: false,
      message: '测试失败：' + e.message,
      checks: [{name: '连接测试', status: 'error', message: e.message}]
    });
    toast('摄像头测试失败：' + e.message);
  }
}

$('#cameraKind').addEventListener('change', () => {
  if ($('#cameraKind').value === 'network' && !editingCameraID) {
    $('#cameraFPS').value = '8';
    $('#cameraPreset').value = 'hikvision';
    cameraAdvancedOpen = false;
  }
  if ($('#cameraKind').value === 'agent' && !editingCameraID) $('#cameraFPS').value = '2';
  if ($('#cameraKind').value === 'local' && !editingCameraID) $('#cameraFPS').value = '30';
  updateCameraFormVisibility();
});

function randomCameraAgentSecret() {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
}

$('#generateCameraAgentSecret').addEventListener('click', () => {
  $('#cameraAgentSecret').value = randomCameraAgentSecret();
  toast('已生成新的 Agent 连接密钥，请同步到教室 Camera Agent 配置');
});

$('#copyCameraAgentConfig').addEventListener('click', async () => {
  const agentID = $('#cameraAgentID').value.trim();
  const secret = $('#cameraAgentSecret').value.trim();
  if (!agentID || !secret) {
    toast('请先填写 Agent ID，并生成或填写连接密钥');
    return;
  }
  const config = {
    server_url: location.origin,
    agent_id: agentID,
    agent_secret: secret,
    fps: Number($('#cameraFPS').value || 2),
    server_tls_insecure: false,
    camera: {
      protocol: 'http_snapshot',
      stream_url: '',
      snapshot_url: 'http://192.168.1.64/ISAPI/Streaming/channels/101/picture',
      username: 'admin',
      password: '请填写摄像头密码',
      auth_mode: 'digest',
      timeout_ms: 3000,
      tls_insecure: false
    }
  };
  try {
    await navigator.clipboard.writeText(JSON.stringify(config, null, 2));
    toast('Agent 配置模板已复制');
  } catch (e) {
    toast('复制失败：' + e.message);
  }
});
function markCameraTestStale() {
  const headline = $('#cameraTestHeadline');
  if (!headline || headline.textContent === '尚未测试连接') return;
  resetCameraTestPanel('摄像头参数已修改，请重新点击“测试连接”确认当前设置。');
}

$('#cameraForm').addEventListener('input', markCameraTestStale);
$('#cameraForm').addEventListener('change', markCameraTestStale);
$('#cameraIP').addEventListener('input', () => {
  if ($('#cameraPreset').value !== 'custom') applyCameraPreset();
});
$('#cameraPreset').addEventListener('change', () => {
  cameraAdvancedOpen = $('#cameraPreset').value === 'custom';
  if ($('#cameraPreset').value !== 'custom') {
    $('#cameraAuthMode').value = 'auto';
    applyCameraPreset();
  }
  updateCameraFormVisibility();
});
$('#cameraAdvancedToggle').addEventListener('click', () => {
  cameraAdvancedOpen = !cameraAdvancedOpen;
  updateCameraFormVisibility();
});
$('#cameraProtocol').addEventListener('change', updateCameraFormVisibility);
$('#testCameraConfig').addEventListener('click', testCurrentCameraConfig);
$('#refreshLocalCameras').addEventListener('click', () => refreshLocalCameraDevices(true).catch(e => toast(e.message)));
$('#cancelCameraEdit').addEventListener('click', resetCameraForm);

$('#cameraForm').addEventListener('submit', async event => {
  event.preventDefault();

  try {
    const payload = cameraFormPayload();
    delete payload.camera_id;
    if (editingCameraID) {
      await api(`/api/cameras/${editingCameraID}`, {
        method: 'PUT',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload)
      });
      toast('摄像头设置已保存');
    } else {
      await api('/api/cameras', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload)
      });
      toast('摄像头已添加');
    }
    if (cameraOpen) stopCamera();
    resetCameraForm();
    await loadCameras();
  } catch (e) {
    toast(e.message);
  }
});

resetCameraForm();
