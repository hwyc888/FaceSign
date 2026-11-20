const sidebarToggle = $('#sidebarToggle');
const sidebarStorageKey = 'facesign.sidebar.collapsed';

function setSidebarCollapsed(collapsed) {
  const shell = $('#appShell');
  if (!shell) return;
  shell.classList.toggle('sidebar-collapsed', Boolean(collapsed));
  if (!sidebarToggle) return;
  sidebarToggle.textContent = collapsed ? '›' : '‹';
  sidebarToggle.setAttribute('aria-expanded', String(!collapsed));
  sidebarToggle.setAttribute('aria-label', collapsed ? '展开导航栏' : '收缩导航栏');
  sidebarToggle.title = collapsed ? '展开导航栏' : '收缩导航栏';
}

try {
  setSidebarCollapsed(localStorage.getItem(sidebarStorageKey) === 'true');
} catch {
  setSidebarCollapsed(false);
}

sidebarToggle?.addEventListener('click', () => {
  const collapsed = !$('#appShell').classList.contains('sidebar-collapsed');
  setSidebarCollapsed(collapsed);
  try {
    localStorage.setItem(sidebarStorageKey, String(collapsed));
  } catch {
    // Sidebar remains usable when browser storage is unavailable.
  }
});

let statusRefreshTimer = null;

function startStatusRefresh() {
  if (statusRefreshTimer !== null) return;
  loadHealth();
  statusRefreshTimer = window.setInterval(loadHealth, 5000);
}

function stopStatusRefresh() {
  if (statusRefreshTimer === null) return;
  window.clearInterval(statusRefreshTimer);
  statusRefreshTimer = null;
}

async function showPage(name, options = {}) {
  if (name !== 'face-compare' && typeof stopFaceCompareCamera === 'function') stopFaceCompareCamera();
  if (name === 'face-compare' && typeof stopCamera === 'function') await stopCamera().catch(() => {});
  if (name !== 'status') stopStatusRefresh();
  if (name !== 'checkin' && typeof stopAutoRecognition === 'function') {
    stopAutoRecognition();
  }

  $$('.nav').forEach(b => b.classList.toggle('active', b.dataset.page === name));
  $$('.page').forEach(p => p.classList.remove('active'));
  $('#page-' + name).classList.add('active');
  $('#pageTitle').textContent = titles[name][0];
  $('#pageHint').textContent = titles[name][1];

  if (name === 'checkin') {
    const autoStart = typeof enterCheckinPageAutoStart === 'function'
      ? enterCheckinPageAutoStart()
      : Promise.resolve();
    const classesLoaded = typeof loadClasses === 'function' ? await loadClasses() : false;
    if (classesLoaded && typeof loadCheckinSeatBoard === 'function') {
      await loadCheckinSeatBoard();
    }
    await autoStart;
  }
  if (name === 'students') {
    await loadStudents();
    startCamera().catch(() => {});
  }
  if (name === 'seating') {
    await loadClasses();
    if (!options.skipSeatEditorReload && Number(window.classSeatEditorID || 0) > 0 && !$('#classSeatEditor').classList.contains('hidden')) {
      await reloadClassSeatEditor();
    }
  }
  if (name === 'attendance') loadAttendance();
  if (name === 'face-compare' && typeof resetFaceComparePage === 'function') resetFaceComparePage();
  if (name === 'users') await loadUsers();
  if (name === 'settings') {
    await Promise.all([loadClasses(), loadCameras(), loadAppSettings(), loadRecognitionStats()]);
    if (typeof currentUser !== 'undefined' && currentUser && currentUser.is_admin &&
        typeof loadAttendanceSnapshotCache === 'function') {
      await loadAttendanceSnapshotCache();
    }
    if (typeof currentUser !== 'undefined' && currentUser && currentUser.is_admin &&
        typeof loadRecognitionDebugThresholds === 'function') {
      await loadRecognitionDebugThresholds();
    }
    refreshLocalCameraDevices(false).catch(() => {});
  }
  if (name === 'status') startStatusRefresh();
}

$$('.nav').forEach(b => b.addEventListener('click', () => showPage(b.dataset.page)));

async function loadHealth() {
  try {
    const h = await api('/api/health');
    $('#healthModelName').textContent = h.engine || '人脸识别模型未上报';
    $('#healthModelParams').textContent = String(h.device || 'CPU').toUpperCase() + ' · 匹配阈值 ' + Number(h.match_threshold || 0).toFixed(2) + ' · ' + h.students + ' 人';
    renderSystemResources(h.resources || {});
    renderRecognitionMetrics(h.recognition_metrics || {});
  } catch (e) {
    $('#healthModelName').textContent = 'FaceSign 服务异常';
    $('#healthModelParams').textContent = e.message || '无法读取模型信息';
    const cpu = $('#systemCpuValue');
    if (cpu) cpu.textContent = '暂不可用';
  }
}

function formatSystemBytes(value) {
  const bytes = Number(value) || 0;
  if (bytes >= 1024 ** 3) return (bytes / (1024 ** 3)).toFixed(1) + ' GB';
  if (bytes >= 1024 ** 2) return (bytes / (1024 ** 2)).toFixed(0) + ' MB';
  return bytes.toLocaleString() + ' B';
}

function renderSystemResources(resources) {
  const cpu = $('#systemCpuValue');
  const cpuBar = $('#systemCpuBar');
  if (cpu) cpu.textContent = resources.cpu_available
    ? Number(resources.cpu_percent).toFixed(1) + '%'
    : '采样中';
  if (cpuBar) cpuBar.style.width = resources.cpu_available
    ? Math.max(0, Math.min(100, Number(resources.cpu_percent))) + '%'
    : '0%';

  const memory = $('#systemMemoryValue');
  const memoryDetail = $('#systemMemoryDetail');
  const memoryBar = $('#systemMemoryBar');
  if (memory) memory.textContent = resources.memory_available
    ? Number(resources.memory_percent).toFixed(1) + '%'
    : '暂不可用';
  if (memoryDetail) memoryDetail.textContent = resources.memory_available
    ? formatSystemBytes(resources.memory_used_bytes) + ' / ' + formatSystemBytes(resources.memory_total_bytes)
    : '无法读取系统内存信息';
  if (memoryBar) memoryBar.style.width = resources.memory_available
    ? Math.max(0, Math.min(100, Number(resources.memory_percent))) + '%'
    : '0%';
}

function renderRecognitionMetrics(metrics) {
  const rate = metrics.success_rate_percent;
  const rateElement = $('#recognitionRateValue');
  const detail = $('#recognitionRateDetail');
  const detected = Number(metrics.detected_faces) || 0;
  const matched = Number(metrics.matched_faces) || 0;
  const successful = Number(metrics.successful_faces) || 0;
  if (rateElement) rateElement.textContent = rate == null ? '--' : Number(rate).toFixed(1) + '%';
  if (detail) detail.textContent = detected > 0
    ? '最近 5 分钟 · 通过 ' + successful + ' / 检测 ' + detected + ' 张 · 身份匹配 ' + matched
    : '最近 5 分钟 · 等待识别数据';
  $('#recognitionChartCounts').textContent = '签到通过 ' + successful + ' / 检测 ' + detected + ' 张人脸';

  const timeline = Array.isArray(metrics.timeline) ? metrics.timeline : [];
  const line = $('#recognitionRateLine');
  const dots = $('#recognitionRateDots');
  const labels = $('#recognitionRateLabels');
  const empty = $('#recognitionChartEmpty');
  if (!line || !dots || !labels || !empty) return;
  line.setAttribute('d', '');
  dots.replaceChildren();
  labels.replaceChildren();

  const usable = timeline.map((point, index) => {
    if (point.success_rate_percent == null) return null;
    return {
      x: timeline.length <= 1 ? 54 : 54 + index * (636 / (timeline.length - 1)),
      y: 184 - Math.max(0, Math.min(100, Number(point.success_rate_percent))) * 1.6,
      rate: Number(point.success_rate_percent),
      time: point.time || ''
    };
  });
  const segments = [];
  let segment = [];
  usable.forEach(point => {
    if (point) segment.push(point);
    else if (segment.length) { segments.push(segment); segment = []; }
  });
  if (segment.length) segments.push(segment);
  const path = segments.map(points => points.map((point, index) =>
    (index === 0 ? 'M ' : 'L ') + point.x.toFixed(1) + ' ' + point.y.toFixed(1)
  ).join(' ')).join(' ');
  line.setAttribute('d', path);

  const ns = 'http://www.w3.org/2000/svg';
  usable.forEach((point, index) => {
    const label = document.createElementNS(ns, 'text');
    const x = timeline.length <= 1 ? 54 : 54 + index * (636 / (timeline.length - 1));
    label.setAttribute('x', x);
    label.setAttribute('y', '213');
    label.setAttribute('text-anchor', 'middle');
    label.textContent = timeline[index].time || '';
    labels.appendChild(label);
    if (!point) return;
    const dot = document.createElementNS(ns, 'circle');
    dot.setAttribute('cx', point.x);
    dot.setAttribute('cy', point.y);
    dot.setAttribute('r', '5');
    dot.setAttribute('class', 'system-chart-dot');
    dot.setAttribute('aria-label', point.rate.toFixed(1) + '%');
    dots.appendChild(dot);
  });
  empty.classList.toggle('hidden', segments.length > 0);
}
