// Administrator-facing navigation and management pages.
// Loaded after app.js so these focused admin functions replace the compact first-pass UI.

const adminSectionLabels = {
  teachers: '教师管理',
  classes: '班级管理',
  students: '学生管理',
  courses: '课程管理',
  enrollment: '课程名单',
  schedules: '课表管理'
};

function renderShell() {
  if (state.eventSource) state.eventSource.close();
  if (state.dashboardTimer) clearInterval(state.dashboardTimer);
  const adminNav = state.me.role === 'admin' ? `
    <button class="btn ghost" data-admin-nav="teachers">教师管理</button>
    <button class="btn ghost" data-admin-nav="classes">班级管理</button>
    <button class="btn ghost" data-admin-nav="students">学生管理</button>
    <button class="btn ghost" data-admin-nav="courses">课程管理</button>
    <button class="btn ghost" data-admin-nav="enrollment">课程名单</button>
    <button class="btn ghost" data-admin-nav="schedules">课表管理</button>` : '';

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
    ['schedules', '课表管理', counts.schedules]
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
    const [users, classes, students, courses, schedules] = await Promise.all([
      api('/api/users'), api('/api/classes'), api('/api/students'), api('/api/courses'), api('/api/schedules')
    ]);

    let enrolled = [];
    if (state.adminSection === 'enrollment' && courses.length > 0) {
      const validCourse = courses.some(course => course.id === Number(state.adminCourseID));
      if (!validCourse) state.adminCourseID = courses[0].id;
      enrolled = await api(`/api/courses/${Number(state.adminCourseID)}/students`);
    }

    const content = adminSectionContent({ users, classes, students, courses, schedules, enrolled });
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
    case 'teachers':
    default: return adminUsers(data.users);
  }
}

function adminEnrollmentManager(courses, classes, students, enrolled) {
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
