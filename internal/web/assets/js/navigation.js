async function showPage(name) {
  $$('.nav').forEach(b => b.classList.toggle('active', b.dataset.page === name));
  $$('.page').forEach(p => p.classList.remove('active'));
  $('#page-' + name).classList.add('active');
  $('#pageTitle').textContent = titles[name][0];
  $('#pageHint').textContent = titles[name][1];

  if (name === 'students') {
    await loadStudents();
    startCamera().catch(() => {});
  }
  if (name === 'attendance') loadAttendance();
  if (name === 'settings') loadClasses();
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
