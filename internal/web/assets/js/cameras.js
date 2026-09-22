let editingCameraID = 0;
let localCameraDevices = [];
let cameraTestPreviewURL = null;

function cameraTypeLabel(camera) {
  if (camera.kind === 'local') return '本机 / USB';
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
  return camera.snapshot_url || camera.stream_url || '-';
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
      <td>${camera.kind === 'network' ? esc(camera.auth_mode === 'digest' ? 'Digest' : camera.auth_mode === 'basic' ? 'Basic' : '无认证') : '-'}</td>
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
  const network = $('#cameraKind').value === 'network';
  $$('[data-camera-local]').forEach(el => el.classList.toggle('hidden', network));
  $$('[data-camera-network]').forEach(el => el.classList.toggle('hidden', !network));
  const protocol = $('#cameraProtocol').value;
  $('#cameraStreamGroup').classList.toggle('hidden', !network || protocol === 'http_snapshot');
  $('#cameraSnapshotGroup').classList.toggle('hidden', !network);
  $('#cameraSnapshotHelp').textContent = protocol === 'rtsp'
    ? 'RTSP用于保存主/子码流参数；FaceSign识别需要填写同一摄像机的HTTP/HTTPS抓图地址。'
    : protocol === 'mjpeg'
      ? 'MJPEG可直接取帧；如另有JPEG抓图地址，填写后会优先使用抓图地址。'
      : '填写返回单张JPEG/PNG图片的HTTP/HTTPS地址。';
}

function resetCameraForm() {
  editingCameraID = 0;
  $('#cameraForm').reset();
  $('#cameraEditID').value = '';
  $('#cameraKind').value = 'local';
  $('#cameraProtocol').value = 'http_snapshot';
  $('#cameraAuthMode').value = 'none';
  $('#cameraWidth').value = '1280';
  $('#cameraHeight').value = '720';
  $('#cameraFPS').value = '30';
  $('#cameraTimeout').value = '3000';
  $('#cameraFormTitle').textContent = '添加摄像头';
  $('#saveCamera').textContent = '添加摄像头';
  $('#cancelCameraEdit').classList.add('hidden');
  $('#cameraClearPasswordWrap').classList.add('hidden');
  $('#cameraPassword').placeholder = '网络摄像头密码';
  renderLocalCameraDeviceOptions('');
  updateCameraFormVisibility();
}

function editCamera(id) {
  const camera = camerasCache.find(item => item.id === id);
  if (!camera) return;
  editingCameraID = id;
  $('#cameraEditID').value = String(id);
  $('#cameraName').value = camera.name;
  $('#cameraKind').value = camera.kind;
  renderLocalCameraDeviceOptions(camera.device_id || '');
  $('#cameraProtocol').value = camera.protocol === 'browser' ? 'http_snapshot' : camera.protocol;
  $('#cameraStreamURL').value = camera.stream_url || '';
  $('#cameraSnapshotURL').value = camera.snapshot_url || '';
  $('#cameraUsername').value = camera.username || '';
  $('#cameraPassword').value = '';
  $('#cameraPassword').placeholder = camera.has_password ? '已保存密码，留空不修改' : '网络摄像头密码';
  $('#cameraClearPassword').checked = false;
  $('#cameraClearPasswordWrap').classList.toggle('hidden', !camera.has_password);
  $('#cameraAuthMode').value = camera.auth_mode || 'none';
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
    if (camera.kind === 'local') {
      const testStream = await navigator.mediaDevices.getUserMedia(localVideoConstraints(camera));
      const settings = testStream.getVideoTracks()[0]?.getSettings?.() || {};
      testStream.getTracks().forEach(track => track.stop());
      $('#cameraTestResult').textContent = `本机摄像头连接成功：${settings.width || camera.width}×${settings.height || camera.height}，${Math.round(settings.frameRate || camera.fps)} FPS`;
      $('#cameraTestPreview').classList.add('hidden');
    } else {
      const blob = await fetchCameraFrameBlob(camera.id);
      if (cameraTestPreviewURL) URL.revokeObjectURL(cameraTestPreviewURL);
      cameraTestPreviewURL = URL.createObjectURL(blob);
      $('#cameraTestPreview').src = cameraTestPreviewURL;
      $('#cameraTestPreview').classList.remove('hidden');
      $('#cameraTestResult').textContent = `网络摄像头“${camera.name}”连接及抓图成功。`;
    }
    toast('摄像头测试成功');
  } catch (e) {
    $('#cameraTestResult').textContent = '测试失败：' + e.message;
    toast('摄像头测试失败：' + e.message);
  }
}

$('#cameraKind').addEventListener('change', () => {
  if ($('#cameraKind').value === 'network' && !editingCameraID) $('#cameraFPS').value = '5';
  if ($('#cameraKind').value === 'local' && !editingCameraID) $('#cameraFPS').value = '30';
  updateCameraFormVisibility();
});
$('#cameraProtocol').addEventListener('change', updateCameraFormVisibility);
$('#refreshLocalCameras').addEventListener('click', () => refreshLocalCameraDevices(true).catch(e => toast(e.message)));
$('#cancelCameraEdit').addEventListener('click', resetCameraForm);

$('#cameraForm').addEventListener('submit', async event => {
  event.preventDefault();
  const kind = $('#cameraKind').value;
  const payload = {
    name: $('#cameraName').value.trim(),
    kind,
    device_id: kind === 'local' ? $('#cameraDevice').value : '',
    protocol: kind === 'local' ? 'browser' : $('#cameraProtocol').value,
    stream_url: kind === 'network' ? $('#cameraStreamURL').value.trim() : '',
    snapshot_url: kind === 'network' ? $('#cameraSnapshotURL').value.trim() : '',
    username: kind === 'network' ? $('#cameraUsername').value.trim() : '',
    auth_mode: kind === 'network' ? $('#cameraAuthMode').value : 'none',
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

  try {
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
