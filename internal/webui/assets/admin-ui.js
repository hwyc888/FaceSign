// Administrator-facing navigation and management pages.
// Loaded after app.js so these focused admin functions replace the compact first-pass UI.

const adminSectionLabels = {
  teachers: '教师管理',
  classes: '班级管理',
  students: '学生管理',
  courses: '课程管理',
  enrollment: '课程名单',
  schedules: '课表管理',
  face: '人脸识别设置'
};

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function renderShell() {
  if (state.eventSource) state.eventSource.close();
  if (state.dashboardTimer) clearInterval(state.dashboardTimer);
  const adminNav = state.me.role === 'admin' ? `
    <button class="btn ghost" data-admin-nav="teachers">教师管理</button>
    <button class="btn ghost" data-admin-nav="classes">班级管理</button>
    <button class="btn ghost" data-admin-nav="students">学生管理</button>
    <button class="btn ghost" data-admin-nav="courses">课程管理</button>
    <button class="btn ghost" data-admin-nav="enrollment">课程名单</button>
    <button class="btn ghost" data-admin-nav="schedules">课表管理</button>
    <button class="btn ghost" data-admin-nav="face">人脸识别</button>` : '';

  app.innerHTML = `
    <div class="shell">
      <header class="topbar">
        <div><div class="brand">FaceSign 学生刷脸考勤</div><div class="muted">${esc(state.me.display_name)} · ${state.me.role === 'admin' ? '管理员' : '教师'}</div></div>
        <nav class="nav">
          <button class="btn ghost" data-view="dashboard">实时考勤</button>
          ${adminNav}
          <button class="btn ghost" id="leave-btn">登记请假</button>
          <a class="btn ghost" href="/kiosk">刷脸终端</a>
          <button class="btn ghost" id="logout-btn">退出</button>
        </nav>
      </header>
      <main id="main"></main>
    </div>`;

  document.querySelectorAll('[data-view]').forEach(button => button.addEventListener('click', () => showView(button.dataset.view)));
  document.querySelectorAll('[data-admin-nav]').forEach(button => button.addEventListener('click', () => {
    state.adminSection = button.dataset.adminNav;
    showView('admin');
  }));
  document.getElementById('leave-btn').addEventListener('click', openLeaveDialog);
  document.getElementById('logout-btn').addEventListener('click', async () => {
    try { await api('/api/logout', { method: 'POST' }); } catch (_) {}
    location.reload();
  });

  state.eventSource = new EventSource('/api/events');
  state.eventSource.addEventListener('refresh', () => { if (state.view === 'dashboard') loadDashboard(); });
  state.eventSource.onerror = () => {};
  state.dashboardTimer = setInterval(() => { if (state.view === 'dashboard') loadDashboard(false); }, 30000);
  showView('dashboard');
}

function adminSectionNav(counts) {
  const sections = [
    ['teachers', '教师管理', counts.teachers],
    ['classes', '班级管理', counts.classes],
    ['students', '学生管理', counts.students],
    ['courses', '课程管理', counts.courses],
    ['enrollment', '课程名单', null],
    ['schedules', '课表管理', counts.schedules],
    ['face', '人脸识别', null]
  ];
  return `<div class="admin-tabs">${sections.map(([key, label, count]) => `
    <button class="admin-tab ${state.adminSection === key ? 'active' : ''}" data-admin-section="${key}">
      <span>${label}</span>${count === null ? '' : `<b>${count}</b>`}
    </button>`).join('')}</div>`;
}

async function loadAdmin() {
  const main = document.getElementById('main');
  if (!main) return;
  if (state.me.role !== 'admin') {
    main.innerHTML = '<div class="panel error">需要管理员权限。</div>';
    return;
  }

  state.adminSection = state.adminSection || 'teachers';
  main.innerHTML = '<div class="panel loading">正在读取系统管理资料…</div>';

  try {
    const raw = await Promise.all([
      api('/api/users'), api('/api/classes'), api('/api/students'), api('/api/courses'), api('/api/schedules')
    ]);
    const users = asArray(raw[0]);
    const classes = asArray(raw[1]);
    const students = asArray(raw[2]);
    const courses = asArray(raw[3]);
    const schedules = asArray(raw[4]);

    let enrolled = [];
    let faceSettings = null;
    if (state.adminSection === 'enrollment' && courses.length > 0) {
      const validCourse = courses.some(course => course.id === Number(state.adminCourseID));
      if (!validCourse) state.adminCourseID = courses[0].id;
      enrolled = asArray(await api(`/api/courses/${Number(state.adminCourseID)}/students`));
    }
    if (state.adminSection === 'face') {
      faceSettings = await api('/api/settings/face');
    }

    const content = adminSectionContent({ users, classes, students, courses, schedules, enrolled, faceSettings });
    main.innerHTML = `
      <section class="panel admin-header">
        <div class="section-title">
          <div>
            <h2 style="margin:0">基础资料与课表管理</h2>
            <div class="muted">请按顺序建立：教师 → 班级 → 学生 → 课程 → 课程名单 → 课表</div>
          </div>
          <span class="admin-current">${adminSectionLabels[state.adminSection]}</span>
        </div>
        ${adminSectionNav({ teachers: users.filter(x => x.role === 'teacher').length, classes: classes.length, students: students.length, courses: courses.length, schedules: schedules.length })}
      </section>
      <div id="admin-section-content">${content}</div>`;

    document.querySelectorAll('[data-admin-section]').forEach(button => button.addEventListener('click', () => {
      state.adminSection = button.dataset.adminSection;
      loadAdmin();
    }));
    bindAdminForms();
  } catch (error) {
    main.innerHTML = `<div class="panel error">${esc(error.message)}</div>`;
  }
}

function adminSectionContent(data) {
  switch (state.adminSection) {
    case 'classes': return adminClasses(data.classes);
    case 'students': return adminStudents(data.classes, data.students);
    case 'courses': return adminCourses(data.users, data.courses);
    case 'enrollment': return adminEnrollmentManager(data.courses, data.classes, data.students, data.enrolled);
    case 'schedules': return adminSchedules(data.courses, data.schedules);
    case 'face': return adminFaceSettings(data.faceSettings);
    case 'teachers':
    default: return adminUsers(data.users);
  }
}

function adminFaceSettings(settings) {
  if (!settings) return '<section class="panel error">无法读取人脸识别设置。</section>';
  const configured = settings.api_key_configured;
  const ready = settings.provider === 'localcpu' || (settings.provider === 'compreface' && configured);
  const serviceURL = settings.service_url || (settings.provider === 'localcpu' ? 'http://127.0.0.1:18081' : 'http://127.0.0.1:8000');
  return `<section class="panel">
    <div class="section-title">
      <div><h3 style="margin:0">人脸识别设置</h3><div class="muted">推荐使用本地 CPU 引擎：不需要 GPU、CUDA 或 Docker。设置保存在 FaceSign 数据库中，保存后立即生效。</div></div>
      <span class="badge ${ready ? 'on_time' : 'pending'}">${ready ? '已配置' : '未启用'}</span>
    </div>
    <form id="face-settings-form">
      <div class="form-grid">
        <div class="field"><label>人脸识别引擎</label><select name="provider" id="face-provider-select">
          <option value="disabled" ${settings.provider === 'disabled' ? 'selected' : ''}>停用</option>
          <option value="localcpu" ${settings.provider === 'localcpu' ? 'selected' : ''}>本地 CPU 引擎（推荐，无需 GPU / Docker）</option>
          <option value="compreface" ${settings.provider === 'compreface' ? 'selected' : ''}>CompreFace</option>
        </select></div>
        <div class="field"><label>服务地址</label><input name="service_url" id="face-service-url" value="${esc(serviceURL)}" placeholder="http://127.0.0.1:18081"></div>
        <div class="field" id="face-api-key-field"><label>API Key（仅 CompreFace）</label><input type="password" name="api_key" autocomplete="new-password" placeholder="${configured ? '已保存；留空表示不修改' : 'CompreFace 才需要填写'}"></div>
        <div class="field"><label>识别相似度阈值</label><input type="number" name="similarity" min="0" max="1" step="0.01" value="${Number(settings.similarity ?? 0.78)}"></div>
        <div class="field"><label>人脸检测阈值</label><input type="number" name="detection_threshold" min="0" max="1" step="0.01" value="${Number(settings.detection_threshold ?? 0.80)}"></div>
      </div>
      <div class="actions" style="margin-top:12px">
        <button class="btn primary" type="submit">保存并立即应用</button>
        <button class="btn good" type="button" id="face-check-btn">检测服务</button>
        ${settings.provider === 'compreface' ? `<a class="btn" href="${esc(serviceURL)}" target="_blank" rel="noopener">打开 CompreFace 控制台</a>` : ''}
      </div>
    </form>
    <div id="face-check-result" class="face-status-box">
      <div class="muted">保存设置后点击“检测服务”。本地 CPU 引擎不需要 API Key。</div>
    </div>
  </section>
  <section class="panel">
    <h3>一键部署本地 CPU 人脸引擎</h3>
    <p class="muted">发布包内提供 Windows / Linux 本地 CPU 引擎。使用 OpenCV YuNet + SFace ONNX 模型，只使用 CPU，不需要独立显卡、CUDA、Docker 或 Docker Desktop。</p>
    <div class="deploy-grid">
      <div class="deploy-card"><b>Windows</b><code>face-engine\\install-localcpu.ps1</code><span>管理员 PowerShell 运行；自动准备独立 Python 运行环境、CPU 模型并注册开机启动任务。</span></div>
      <div class="deploy-card"><b>Linux</b><code>face-engine/install-localcpu.sh</code><span>自动创建 venv、安装 CPU 依赖和 systemd 服务。</span></div>
    </div>
    <div class="notice">部署完成后选择“本地 CPU 引擎”，服务地址填写 http://127.0.0.1:18081，保存后点击“检测服务”即可。不需要 API Key。</div>
  </section>`;
}

function adminEnrollmentManager(courses, classes, students, enrolled) {
  courses = asArray(courses);
  classes = asArray(classes);
  students = asArray(students);
  enrolled = asArray(enrolled);

  if (courses.length === 0) {
    return '<section class="panel"><h3>课程学生名单</h3><div class="notice">请先到“课程管理”建立课程。</div></section>';
  }
  if (students.length === 0) {
    return '<section class="panel"><h3>课程学生名单</h3><div class="notice">请先到“学生管理”建立学生。</div></section>';
  }

  const enrolledIDs = new Set(enrolled.map(item => item.id));
  const available = students.filter(item => !enrolledIDs.has(item.id));
  const currentCourse = courses.find(item => item.id === Number(state.adminCourseID)) || courses[0];

  return `<section class="panel">
    <div class="section-title">
      <div><h3 style="margin:0">课程学生名单</h3><div class="muted">选择课程后可查看完整名单，并添加或移除学生。</div></div>
      <div class="field compact-field"><label>当前课程</label><select id="enrollment-course">${optionList(courses, 'id', x => `${x.name} / ${x.teacher_name}`)}</select></div>
    </div>

    <div class="enrollment-summary">
      <div><b>${esc(currentCourse.name)}</b><span>当前 ${enrolled.length} 名学生</span></div>
    </div>

    <form id="enroll-form">
      <input type="hidden" name="course_id" value="${currentCourse.id}">
      <div class="form-grid">
        <div class="field"><label>添加学生</label><select name="student_id" ${available.length ? 'required' : 'disabled'}>
          ${available.length ? `<option value="">请选择</option>${optionList(available, 'id', x => `${x.class_name} / ${x.name} / ${x.student_no}`)}` : '<option>该课程已包含全部学生</option>'}
        </select></div>
        <button class="btn primary" ${available.length ? '' : 'disabled'}>加入课程</button>
      </div>
    </form>

    <div class="table-wrap"><table>
      <thead><tr><th>学号</th><th>姓名</th><th>班级</th><th>状态</th><th>操作</th></tr></thead>
      <tbody>${enrolled.length ? enrolled.map(item => `<tr>
        <td>${esc(item.student_no)}</td><td>${esc(item.name)}</td><td>${esc(item.class_name)}</td>
        <td>${item.active ? '在读' : '停用'}</td>
        <td><button class="btn danger" data-remove-enrollment="${item.id}" data-remove-name="${esc(item.name)}">移出课程</button></td>
      </tr>`).join('') : '<tr><td colspan="5" class="muted">该课程还没有学生，请从上方添加。</td></tr>'}</tbody>
    </table></div>
  </section>`;
}

function bindAdminForms() {
  const submit = (id, fn) => document.getElementById(id)?.addEventListener('submit', async event => {
    event.preventDefault();
    try {
      await fn(Object.fromEntries(new FormData(event.currentTarget)));
      notify('保存成功', 'success');
      await loadAdmin();
    } catch (error) {
      notify(error.message, 'error');
    }
  });

  submit('user-form', data => api('/api/users', { method: 'POST', body: data }));
  submit('class-form', data => api('/api/classes', { method: 'POST', body: data }));
  submit('student-form', data => api('/api/students', { method: 'POST', body: { ...data, class_id: Number(data.class_id) } }));
  submit('course-form', data => api('/api/courses', { method: 'POST', body: { ...data, teacher_id: Number(data.teacher_id) } }));
  submit('enroll-form', data => api(`/api/courses/${Number(data.course_id)}/students`, { method: 'POST', body: { student_id: Number(data.student_id) } }));
  submit('schedule-form', data => api('/api/schedules', { method: 'POST', body: { course_id: Number(data.course_id), classroom: data.classroom, weekday: Number(data.weekday), start_minute: timeToMinutes(data.start), end_minute: timeToMinutes(data.end), grace_minutes: Number(data.grace_minutes), checkin_before_minutes: Number(data.checkin_before_minutes), enabled: true } }));
  submit('face-settings-form', data => api('/api/settings/face', { method: 'PUT', body: {
    provider: data.provider,
    service_url: data.service_url,
    api_key: data.api_key,
    similarity: Number(data.similarity),
    detection_threshold: Number(data.detection_threshold)
  } }));

  const faceProviderSelect = document.getElementById('face-provider-select');
  const faceServiceURL = document.getElementById('face-service-url');
  const faceAPIKeyField = document.getElementById('face-api-key-field');
  const faceSimilarity = document.querySelector('#face-settings-form [name="similarity"]');
  const syncFaceProviderUI = (changed = false) => {
    if (!faceProviderSelect) return;
    const localCPU = faceProviderSelect.value === 'localcpu';
    if (faceAPIKeyField) faceAPIKeyField.style.display = faceProviderSelect.value === 'compreface' ? '' : 'none';
    if (changed && faceServiceURL) {
      faceServiceURL.value = localCPU ? 'http://127.0.0.1:18081' : (faceProviderSelect.value === 'compreface' ? 'http://127.0.0.1:8000' : faceServiceURL.value);
    }
    if (changed && localCPU && faceSimilarity && Number(faceSimilarity.value) === 0.78) {
      faceSimilarity.value = '0.72';
    }
  };
  if (faceProviderSelect) {
    syncFaceProviderUI(false);
    faceProviderSelect.addEventListener('change', () => syncFaceProviderUI(true));
  }

  const faceCheck = document.getElementById('face-check-btn');
  if (faceCheck) {
    faceCheck.addEventListener('click', async () => {
      const box = document.getElementById('face-check-result');
      faceCheck.disabled = true;
      box.innerHTML = '<div class="notice">正在检测人脸识别服务…</div>';
      try {
        const status = await api('/api/settings/face/check', { method: 'POST' });
        box.innerHTML = `<div class="${status.reachable ? 'success' : 'error'}"><b>${esc(status.message)}</b>${status.detail ? `<div class="face-detail">${esc(status.detail)}</div>` : ''}</div>`;
      } catch (error) {
        box.innerHTML = `<div class="error">检测失败：${esc(error.message)}</div>`;
      } finally {
        faceCheck.disabled = false;
      }
    });
  }

  const courseSelect = document.getElementById('enrollment-course');
  if (courseSelect) {
    courseSelect.value = String(state.adminCourseID || '');
    courseSelect.addEventListener('change', () => {
      state.adminCourseID = Number(courseSelect.value);
      loadAdmin();
    });
  }

  document.querySelectorAll('[data-remove-enrollment]').forEach(button => button.addEventListener('click', async () => {
    const studentID = Number(button.dataset.removeEnrollment);
    const name = button.dataset.removeName || '该学生';
    if (!confirm(`确定将 ${name} 从当前课程名单中移除？`)) return;
    try {
      await api(`/api/courses/${Number(state.adminCourseID)}/students/${studentID}`, { method: 'DELETE' });
      notify('已从课程名单移除', 'success');
      await loadAdmin();
    } catch (error) {
      notify(error.message, 'error');
    }
  }));

  document.querySelectorAll('[data-delete-class]').forEach(button => button.addEventListener('click', () => removeEntity(`/api/classes/${button.dataset.deleteClass}`, '班级')));
  document.querySelectorAll('[data-delete-student]').forEach(button => button.addEventListener('click', () => removeEntity(`/api/students/${button.dataset.deleteStudent}`, '学生')));
  document.querySelectorAll('[data-delete-course]').forEach(button => button.addEventListener('click', () => removeEntity(`/api/courses/${button.dataset.deleteCourse}`, '课程')));
  document.querySelectorAll('[data-delete-schedule]').forEach(button => button.addEventListener('click', () => removeEntity(`/api/schedules/${button.dataset.deleteSchedule}`, '课表')));
  document.querySelectorAll('[data-face]').forEach(button => button.addEventListener('click', () => openFaceDialog(Number(button.dataset.face), button.dataset.faceName)));
}
