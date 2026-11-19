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

async function showPage(name, options = {}) {
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
  if (name === 'users') await loadUsers();
  if (name === 'settings') {
    await Promise.all([loadClasses(), loadCameras(), loadAppSettings(), loadRecognitionStats()]);
    refreshLocalCameraDevices(false).catch(() => {});
  }
  if (name === 'status') loadHealth();
}

$$('.nav').forEach(b => b.addEventListener('click', () => showPage(b.dataset.page)));

async function loadHealth() {
  try {
    const h = await api('/api/health');
    $('#healthPill').textContent = `CPU | ${h.engine} | ${h.students} 人`;
    $('#statusJson').textContent = JSON.stringify(h, null, 2);
  } catch (e) {
    $('#healthPill').textContent = '服务异常';
    $('#statusJson').textContent = e.message;
  }
}
