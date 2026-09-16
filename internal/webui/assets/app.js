const app = document.getElementById('app');
const state = { me: null, view: 'dashboard', eventSource: null, dashboardTimer: null };

const statusText = { on_time: '准时', late: '迟到', leave: '请假', absent: '旷课', pending: '未签到' };
const weekdayText = ['', '周一', '周二', '周三', '周四', '周五', '周六', '周日'];

function esc(value = '') {
  return String(value).replace(/[&<>'"]/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[ch]));
}

async function api(url, options = {}) {
  const opts = { credentials: 'same-origin', ...options, headers: { ...(options.headers || {}) } };
  if (opts.body && !(opts.body instanceof FormData) && typeof opts.body !== 'string') {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(opts.body);
  }
  const response = await fetch(url, opts);
  if (response.status === 204) return null;
  const type = response.headers.get('content-type') || '';
  const payload = type.includes('application/json') ? await response.json() : await response.text();
  if (!response.ok) {
    const message = payload && payload.error ? payload.error : (typeof payload === 'string' ? payload : `HTTP ${response.status}`);
    const error = new Error(message);
    error.status = response.status;
    throw error;
  }
  return payload;
}

function notify(message, kind = 'notice') {
  let node = document.getElementById('toast');
  if (!node) {
    node = document.createElement('div');
    node.id = 'toast';
    node.style.cssText = 'position:fixed;right:18px;bottom:18px;z-index:9999;max-width:min(420px,90vw)';
    document.body.appendChild(node);
  }
  node.innerHTML = `<div class="${kind}">${esc(message)}</div>`;
  clearTimeout(node._timer);
  node._timer = setTimeout(() => node.innerHTML = '', 3200);
}

async function boot() {
  if (location.pathname === '/kiosk') {
    renderKiosk();
    return;
  }
  try {
    state.me = await api('/api/me');
    renderShell();
  } catch (error) {
    if (error.status !== 401) {
      app.innerHTML = `<div class="auth"><h1>FaceSign</h1><div class="error">${esc(error.message)}</div></div>`;
      return;
    }
    const setup = await api('/api/setup/status');
    renderAuth(setup.setup_required);
  }
}

function renderAuth(setupRequired) {
  app.innerHTML = `
    <main class="auth">
      <h1>${setupRequired ? '初始化 FaceSign' : 'FaceSign 登录'}</h1>
      <p class="muted">学生刷脸考勤与课程实时考勤平台</p>
      <div id="auth-error"></div>
      <form id="auth-form">
        ${setupRequired ? `<div class="field"><label>管理员姓名</label><input name="display_name" required autocomplete="name" placeholder="例如：系统管理员"></div>` : ''}
        <div class="field"><label>用户名</label><input name="username" required autocomplete="username"></div>
        <div class="field"><label>密码</label><input type="password" name="password" required autocomplete="${setupRequired ? 'new-password' : 'current-password'}"></div>
        <button class="btn primary" type="submit">${setupRequired ? '创建管理员并进入系统' : '登录'}</button>
      </form>
      <p style="margin-top:18px"><a href="/kiosk">进入教室刷脸终端</a></p>
    </main>`;
  document.getElementById('auth-form').addEventListener('submit', async event => {
    event.preventDefault();
    const data = Object.fromEntries(new FormData(event.currentTarget));
    const errorBox = document.getElementById('auth-error');
    errorBox.innerHTML = '';
    try {
      if (setupRequired) {
        await api('/api/setup', { method: 'POST', body: data });
      }
      state.me = await api('/api/login', { method: 'POST', body: { username: data.username, password: data.password } });
      renderShell();
    } catch (error) {
      errorBox.innerHTML = `<div class="error">${esc(error.message)}</div>`;
    }
  });
}

function renderShell() {
  if (state.eventSource) state.eventSource.close();
  if (state.dashboardTimer) clearInterval(state.dashboardTimer);
  app.innerHTML = `
    <div class="shell">
      <header class="topbar">
        <div><div class="brand">FaceSign 学生刷脸考勤</div><div class="muted">${esc(state.me.display_name)} · ${state.me.role === 'admin' ? '管理员' : '教师'}</div></div>
        <nav class="nav">
          <button class="btn ghost" data-view="dashboard">实时考勤</button>
          <button class="btn ghost" id="leave-btn">登记请假</button>
          ${state.me.role === 'admin' ? '<button class="btn ghost" data-view="admin">系统管理</button>' : ''}
          <a class="btn ghost" href="/kiosk">刷脸终端</a>
          <button class="btn ghost" id="logout-btn">退出</button>
        </nav>
      </header>
      <main id="main"></main>
    </div>`;

  document.querySelectorAll('[data-view]').forEach(button => button.addEventListener('click', () => showView(button.dataset.view)));
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

function showView(view) {
  state.view = view;
  if (view === 'admin' && state.me.role === 'admin') loadAdmin();
  else loadDashboard();
}

async function loadDashboard(showLoading = true) {
  const main = document.getElementById('main');
  if (!main) return;
  if (showLoading) main.innerHTML = '<div class="panel loading">正在读取实时考勤…</div>';
  try {
    const sessions = await api('/api/dashboard/current');
    if (!sessions || sessions.length === 0) {
      main.innerHTML = `<section class="panel"><h2>实时考勤</h2><div class="notice">当前没有正在进行或刚结束的课程。系统会按照课表自动开启考勤场次。</div></section>`;
      return;
    }
    main.innerHTML = `
      <section class="panel">
        <div class="section-title"><div><h2 style="margin:0">实时考勤</h2><div class="muted">页面通过实时事件自动刷新</div></div><button class="btn" id="refresh-dashboard">刷新</button></div>
        <div class="grid cards">
          ${sessions.map(sessionCard).join('')}
        </div>
      </section>
      <section class="panel" id="records-panel"><div class="muted">点击课程卡片查看学生明细。</div></section>`;
    document.getElementById('refresh-dashboard').addEventListener('click', () => loadDashboard());
    document.querySelectorAll('[data-session]').forEach(card => card.addEventListener('click', () => loadSessionRecords(card.dataset.session)));
  } catch (error) {
    main.innerHTML = `<div class="panel error">${esc(error.message)}</div>`;
  }
}

function sessionCard(session) {
  const start = new Date(session.start_at * 1000).toLocaleTimeString([], { hour:'2-digit', minute:'2-digit' });
  const end = new Date(session.end_at * 1000).toLocaleTimeString([], { hour:'2-digit', minute:'2-digit' });
  return `<button class="card" data-session="${session.id}" style="text-align:left;border:1px solid #e2e8f0">
    <h3>${esc(session.course_name)}</h3>
    <div class="muted">${esc(session.classroom)} · ${start}–${end} · ${session.state === 'open' ? '考勤中' : '已结束'}</div>
    <div class="stats">
      <div class="stat"><b>${session.on_time}</b>准时</div>
      <div class="stat"><b>${session.late}</b>迟到</div>
      <div class="stat"><b>${session.leave}</b>请假</div>
      <div class="stat"><b>${session.absent}</b>旷课</div>
      <div class="stat"><b>${session.pending}</b>未签到</div>
    </div>
  </button>`;
}

async function loadSessionRecords(sessionID) {
  const panel = document.getElementById('records-panel');
  if (!panel) return;
  panel.innerHTML = '<div class="loading">正在读取学生明细…</div>';
  try {
    const rows = await api(`/api/attendance/sessions/${sessionID}/records`);
    panel.innerHTML = `
      <div class="section-title"><h3 style="margin:0">学生考勤明细</h3><span class="muted">教师可人工修正状态</span></div>
      <div class="table-wrap"><table>
        <thead><tr><th>学号</th><th>姓名</th><th>班级</th><th>状态</th><th>签到时间</th><th>相似度</th><th>操作</th></tr></thead>
        <tbody>${rows.map(row => `<tr>
          <td>${esc(row.student_no)}</td><td>${esc(row.student_name)}</td><td>${esc(row.class_name)}</td>
          <td><span class="badge ${esc(row.status)}">${statusText[row.status] || esc(row.status)}</span></td>
          <td>${row.recognized_at ? new Date(row.recognized_at*1000).toLocaleTimeString() : '-'}</td>
          <td>${row.similarity ? (row.similarity*100).toFixed(1)+'%' : '-'}</td>
          <td class="actions">
            <button class="btn" data-manual="on_time" data-student="${row.student_id}">准时</button>
            <button class="btn" data-manual="late" data-student="${row.student_id}">迟到</button>
            <button class="btn" data-manual="leave" data-student="${row.student_id}">请假</button>
            <button class="btn danger" data-manual="absent" data-student="${row.student_id}">旷课</button>
          </td></tr>`).join('')}</tbody>
      </table></div>`;
    panel.querySelectorAll('[data-manual]').forEach(button => button.addEventListener('click', async () => {
      try {
        await api('/api/attendance/manual', { method:'POST', body:{ session_id:Number(sessionID), student_id:Number(button.dataset.student), status:button.dataset.manual, note:'教师人工修正' } });
        notify('考勤状态已更新', 'success');
        await loadSessionRecords(sessionID);
      } catch (error) { notify(error.message, 'error'); }
    }));
  } catch (error) {
    panel.innerHTML = `<div class="error">${esc(error.message)}</div>`;
  }
}

async function loadAdmin() {
  const main = document.getElementById('main');
  main.innerHTML = '<div class="panel loading">正在读取系统资料…</div>';
  try {
    const [users, classes, students, courses, schedules] = await Promise.all([
      api('/api/users'), api('/api/classes'), api('/api/students'), api('/api/courses'), api('/api/schedules')
    ]);
    main.innerHTML = `
      <section class="panel"><div class="section-title"><h2 style="margin:0">系统管理</h2><span class="muted">先建立教师、班级、学生、课程，再配置选课与课表</span></div></section>
      <div class="admin-grid">
        ${adminUsers(users)}
        ${adminClasses(classes)}
        ${adminStudents(classes, students)}
        ${adminCourses(users, courses)}
        ${adminEnrollment(courses, students)}
        ${adminSchedules(courses, schedules)}
      </div>`;
    bindAdminForms();
  } catch (error) {
    main.innerHTML = `<div class="panel error">${esc(error.message)}</div>`;
  }
}

function optionList(items, valueKey, labelFn) {
  return items.map(item => `<option value="${item[valueKey]}">${esc(labelFn(item))}</option>`).join('');
}

function adminUsers(users) {
  return `<section class="panel"><h3>教师 / 管理员</h3>
    <form id="user-form"><div class="form-grid">
      <div class="field"><label>姓名</label><input name="display_name" required></div>
      <div class="field"><label>用户名</label><input name="username" required></div>
      <div class="field"><label>密码</label><input type="password" name="password" required></div>
      <div class="field"><label>角色</label><select name="role"><option value="teacher">教师</option><option value="admin">管理员</option></select></div>
      <button class="btn primary">新增用户</button>
    </div></form>
    <div class="table-wrap"><table><thead><tr><th>姓名</th><th>用户名</th><th>角色</th></tr></thead><tbody>${users.map(x=>`<tr><td>${esc(x.display_name)}</td><td>${esc(x.username)}</td><td>${x.role==='admin'?'管理员':'教师'}</td></tr>`).join('')}</tbody></table></div>
  </section>`;
}

function adminClasses(classes) {
  return `<section class="panel"><h3>班级</h3>
    <form id="class-form" class="form-grid"><div class="field"><label>班级名称</label><input name="name" required></div><button class="btn primary">新增班级</button></form>
    <div class="table-wrap"><table><thead><tr><th>班级</th><th>操作</th></tr></thead><tbody>${classes.map(x=>`<tr><td>${esc(x.name)}</td><td><button class="btn danger" data-delete-class="${x.id}">删除</button></td></tr>`).join('')}</tbody></table></div>
  </section>`;
}

function adminStudents(classes, students) {
  return `<section class="panel"><h3>学生</h3>
    <form id="student-form"><div class="form-grid">
      <div class="field"><label>学号</label><input name="student_no" required></div>
      <div class="field"><label>姓名</label><input name="name" required></div>
      <div class="field"><label>班级</label><select name="class_id" required><option value="">请选择</option>${optionList(classes,'id',x=>x.name)}</select></div>
      <button class="btn primary">新增学生</button>
    </div></form>
    <div class="table-wrap"><table><thead><tr><th>学号</th><th>姓名</th><th>班级</th><th>人脸</th><th>操作</th></tr></thead><tbody>${students.map(x=>`<tr><td>${esc(x.student_no)}</td><td>${esc(x.name)}</td><td>${esc(x.class_name)}</td><td><button class="btn good" data-face="${x.id}" data-face-name="${esc(x.name)}">采集</button></td><td><button class="btn danger" data-delete-student="${x.id}">删除</button></td></tr>`).join('')}</tbody></table></div>
  </section>`;
}

function adminCourses(users, courses) {
  const teachers = users.filter(x => x.role === 'teacher' || x.role === 'admin');
  return `<section class="panel"><h3>课程</h3>
    <form id="course-form"><div class="form-grid">
      <div class="field"><label>课程名称</label><input name="name" required></div>
      <div class="field"><label>任课教师</label><select name="teacher_id" required><option value="">请选择</option>${optionList(teachers,'id',x=>x.display_name)}</select></div>
      <button class="btn primary">新增课程</button>
    </div></form>
    <div class="table-wrap"><table><thead><tr><th>课程</th><th>教师</th><th>操作</th></tr></thead><tbody>${courses.map(x=>`<tr><td>${esc(x.name)}</td><td>${esc(x.teacher_name)}</td><td><button class="btn danger" data-delete-course="${x.id}">删除</button></td></tr>`).join('')}</tbody></table></div>
  </section>`;
}

function adminEnrollment(courses, students) {
  return `<section class="panel"><h3>课程学生名单</h3>
    <form id="enroll-form"><div class="form-grid">
      <div class="field"><label>课程</label><select name="course_id" required><option value="">请选择</option>${optionList(courses,'id',x=>x.name)}</select></div>
      <div class="field"><label>学生</label><select name="student_id" required><option value="">请选择</option>${optionList(students,'id',x=>`${x.class_name} / ${x.name} / ${x.student_no}`)}</select></div>
      <button class="btn primary">加入课程</button>
    </div></form>
    <p class="muted">同一学生重复加入会自动忽略。课程名单可通过课程接口继续扩展批量导入。</p>
  </section>`;
}

function adminSchedules(courses, schedules) {
  return `<section class="panel"><h3>课表</h3>
    <form id="schedule-form"><div class="form-grid">
      <div class="field"><label>课程</label><select name="course_id" required><option value="">请选择</option>${optionList(courses,'id',x=>x.name)}</select></div>
      <div class="field"><label>教室</label><input name="classroom" required placeholder="例如 机房301"></div>
      <div class="field"><label>星期</label><select name="weekday">${[1,2,3,4,5,6,7].map(i=>`<option value="${i}">${weekdayText[i]}</option>`).join('')}</select></div>
      <div class="field"><label>开始时间</label><input type="time" name="start" required></div>
      <div class="field"><label>结束时间</label><input type="time" name="end" required></div>
      <div class="field"><label>迟到宽限(分钟)</label><input type="number" name="grace_minutes" min="0" max="120" value="5"></div>
      <div class="field"><label>提前签到(分钟)</label><input type="number" name="checkin_before_minutes" min="0" max="180" value="20"></div>
      <button class="btn primary">新增课表</button>
    </div></form>
    <div class="table-wrap"><table><thead><tr><th>星期</th><th>课程</th><th>教室</th><th>时间</th><th>宽限</th><th>操作</th></tr></thead><tbody>${schedules.map(x=>`<tr><td>${weekdayText[x.weekday]}</td><td>${esc(x.course_name)}</td><td>${esc(x.classroom)}</td><td>${minutesText(x.start_minute)}–${minutesText(x.end_minute)}</td><td>${x.grace_minutes} 分钟</td><td><button class="btn danger" data-delete-schedule="${x.id}">删除</button></td></tr>`).join('')}</tbody></table></div>
  </section>`;
}

function minutesText(value) {
  const h = Math.floor(value / 60).toString().padStart(2,'0');
  const m = (value % 60).toString().padStart(2,'0');
  return `${h}:${m}`;
}
function timeToMinutes(value) { const [h,m] = value.split(':').map(Number); return h*60+m; }

function bindAdminForms() {
  const submit = (id, fn) => document.getElementById(id)?.addEventListener('submit', async event => {
    event.preventDefault();
    try { await fn(Object.fromEntries(new FormData(event.currentTarget))); notify('保存成功','success'); await loadAdmin(); }
    catch (error) { notify(error.message,'error'); }
  });
  submit('user-form', data => api('/api/users',{method:'POST',body:data}));
  submit('class-form', data => api('/api/classes',{method:'POST',body:data}));
  submit('student-form', data => api('/api/students',{method:'POST',body:{...data,class_id:Number(data.class_id)}}));
  submit('course-form', data => api('/api/courses',{method:'POST',body:{...data,teacher_id:Number(data.teacher_id)}}));
  submit('enroll-form', data => api(`/api/courses/${Number(data.course_id)}/students`,{method:'POST',body:{student_id:Number(data.student_id)}}));
  submit('schedule-form', data => api('/api/schedules',{method:'POST',body:{course_id:Number(data.course_id),classroom:data.classroom,weekday:Number(data.weekday),start_minute:timeToMinutes(data.start),end_minute:timeToMinutes(data.end),grace_minutes:Number(data.grace_minutes),checkin_before_minutes:Number(data.checkin_before_minutes),enabled:true}}));

  document.querySelectorAll('[data-delete-class]').forEach(b=>b.addEventListener('click',()=>removeEntity(`/api/classes/${b.dataset.deleteClass}`,'班级')));
  document.querySelectorAll('[data-delete-student]').forEach(b=>b.addEventListener('click',()=>removeEntity(`/api/students/${b.dataset.deleteStudent}`,'学生')));
  document.querySelectorAll('[data-delete-course]').forEach(b=>b.addEventListener('click',()=>removeEntity(`/api/courses/${b.dataset.deleteCourse}`,'课程')));
  document.querySelectorAll('[data-delete-schedule]').forEach(b=>b.addEventListener('click',()=>removeEntity(`/api/schedules/${b.dataset.deleteSchedule}`,'课表')));
  document.querySelectorAll('[data-face]').forEach(b=>b.addEventListener('click',()=>openFaceDialog(Number(b.dataset.face),b.dataset.faceName)));
}

async function removeEntity(url, label) {
  if (!confirm(`确定删除该${label}？已经产生考勤历史的数据会被数据库保护而拒绝删除。`)) return;
  try { await api(url,{method:'DELETE'}); notify(`${label}已删除`,'success'); await loadAdmin(); }
  catch (error) { notify(error.message,'error'); }
}

async function openFaceDialog(studentID, name) {
  try {
    const status = await api('/api/settings/face/check', { method:'POST' });
    if (!status.reachable) {
      notify(status.message || '人脸识别服务当前不可用，请先完成配置并检测服务。', 'error');
      return;
    }
  } catch (error) {
    notify(`无法检测人脸识别服务：${error.message}`, 'error');
    return;
  }
  const dialog = document.createElement('dialog');
  dialog.innerHTML = `<div class="dialog-body"><div class="section-title"><h3 style="margin:0">采集 ${esc(name)} 的人脸</h3><button class="btn" id="face-close">关闭</button></div><div class="notice">建议光线均匀、正脸无遮挡；可重复采集 2–3 张不同角度样本提高识别稳定性。</div><video class="camera" autoplay playsinline muted></video><div class="actions" style="margin-top:12px"><button class="btn good" id="face-capture">拍照并录入</button></div><div id="face-message"></div></div>`;
  document.body.appendChild(dialog);
  dialog.showModal();
  const video = dialog.querySelector('video');
  let stream;
  const close = () => { if (stream) stream.getTracks().forEach(t=>t.stop()); dialog.close(); dialog.remove(); };
  dialog.querySelector('#face-close').addEventListener('click',close);
  dialog.addEventListener('cancel',event=>{event.preventDefault();close();});
  try { stream = await navigator.mediaDevices.getUserMedia({video:{facingMode:'user',width:{ideal:720}},audio:false}); video.srcObject=stream; }
  catch (error) { dialog.querySelector('#face-message').innerHTML=`<div class="error">无法打开摄像头：${esc(error.message)}</div>`; return; }
  dialog.querySelector('#face-capture').addEventListener('click',async()=>{
    const message=dialog.querySelector('#face-message');message.innerHTML='<div class="notice">正在录入…</div>';
    try {
      const blob=await captureBlob(video);const form=new FormData();form.append('file',blob,'face.jpg');
      await api(`/api/students/${studentID}/face`,{method:'POST',body:form});
      message.innerHTML='<div class="success">人脸样本录入成功，可继续采集下一张。</div>';
    } catch(error){message.innerHTML=`<div class="error">${esc(error.message)}</div>`;}
  });
}

async function openLeaveDialog() {
  let students, schedules;
  try { [students,schedules] = await Promise.all([api('/api/students'),api('/api/schedules')]); }
  catch(error){notify(error.message,'error');return;}
  const dialog=document.createElement('dialog');
  const today=new Date().toISOString().slice(0,10);
  dialog.innerHTML=`<div class="dialog-body"><div class="section-title"><h3 style="margin:0">登记请假</h3><button class="btn" id="leave-close">关闭</button></div><form id="leave-form"><div class="field"><label>学生</label><select name="student_id" required>${optionList(students,'id',x=>`${x.class_name} / ${x.name}`)}</select></div><div class="field"><label>课程课表</label><select name="schedule_id" required>${optionList(schedules,'id',x=>`${weekdayText[x.weekday]} ${minutesText(x.start_minute)} ${x.course_name} / ${x.classroom}`)}</select></div><div class="field"><label>请假日期</label><input type="date" name="leave_date" value="${today}" required></div><div class="field"><label>原因</label><textarea name="reason" rows="3"></textarea></div><button class="btn primary">登记请假</button></form><div id="leave-message"></div></div>`;
  document.body.appendChild(dialog);dialog.showModal();
  const close=()=>{dialog.close();dialog.remove();};dialog.querySelector('#leave-close').addEventListener('click',close);dialog.addEventListener('cancel',e=>{e.preventDefault();close();});
  dialog.querySelector('#leave-form').addEventListener('submit',async event=>{event.preventDefault();const data=Object.fromEntries(new FormData(event.currentTarget));try{await api('/api/leaves',{method:'POST',body:{student_id:Number(data.student_id),schedule_id:Number(data.schedule_id),leave_date:data.leave_date,reason:data.reason}});dialog.querySelector('#leave-message').innerHTML='<div class="success">请假登记成功。</div>';if(state.view==='dashboard')loadDashboard(false);}catch(error){dialog.querySelector('#leave-message').innerHTML=`<div class="error">${esc(error.message)}</div>`;}});
}

function captureBlob(video) {
  return new Promise((resolve,reject)=>{
    if (!video.videoWidth || !video.videoHeight) { reject(new Error('摄像头画面尚未准备好')); return; }
    const max=720, scale=Math.min(1,max/video.videoWidth);
    const canvas=document.createElement('canvas');canvas.width=Math.round(video.videoWidth*scale);canvas.height=Math.round(video.videoHeight*scale);
    canvas.getContext('2d').drawImage(video,0,0,canvas.width,canvas.height);
    canvas.toBlob(blob=>blob?resolve(blob):reject(new Error('无法生成摄像头图片')),'image/jpeg',0.88);
  });
}

async function renderKiosk() {
  app.innerHTML=`<main class="kiosk"><header class="topbar"><div><div class="brand">FaceSign 教室刷脸终端</div><div class="muted">按当前教室课表自动判定准时或迟到</div></div><a class="btn ghost" href="/">教师平台</a></header><section class="panel" id="kiosk-config"><div class="form-grid"><div class="field"><label>教室</label><input id="kiosk-classroom" placeholder="例如 机房301"></div><div class="field"><label>终端密钥</label><input id="kiosk-key" type="password" placeholder="由服务器管理员配置"></div><button class="btn primary" id="kiosk-start">启动摄像头</button></div><div id="secure-warning"></div></section><section class="panel"><div class="video-box"><video id="kiosk-video" autoplay playsinline muted></video><div class="scan-line"></div></div><div class="result" id="kiosk-result">等待启动</div></section></main>`;
  const classroom=document.getElementById('kiosk-classroom'),key=document.getElementById('kiosk-key'),result=document.getElementById('kiosk-result'),video=document.getElementById('kiosk-video'),startButton=document.getElementById('kiosk-start');
  classroom.value=sessionStorage.getItem('facesign_classroom')||'';key.value=sessionStorage.getItem('facesign_kiosk_key')||'';
  if(!window.isSecureContext){document.getElementById('secure-warning').innerHTML='<div class="error">浏览器摄像头通常要求 HTTPS（localhost 例外）。请给 FaceSign 配置 TLS 证书或通过 HTTPS 反向代理访问。</div>';}
  let faceEnabled=true;
  try{const status=await api('/api/kiosk/status');if(!status.face_ready){faceEnabled=false;startButton.disabled=true;result.className='result bad';result.textContent=status.face_message||'人脸识别服务尚未启用或当前不可用，请联系管理员到“人脸识别设置”完成配置并检测服务。';}if(!status.kiosk_key_required)key.closest('.field').style.display='none';}catch(error){faceEnabled=false;startButton.disabled=true;result.className='result bad';result.textContent=error.message;}
  let stream=null,timer=null,busy=false;
  startButton.addEventListener('click',async()=>{
    if(!faceEnabled){notify('人脸识别服务尚未启用或当前不可用，请联系管理员完成配置并检测服务。','error');return;}
    if(!classroom.value.trim()){notify('请先填写教室名称','error');return;}
    sessionStorage.setItem('facesign_classroom',classroom.value.trim());sessionStorage.setItem('facesign_kiosk_key',key.value);
    try{if(stream)stream.getTracks().forEach(t=>t.stop());stream=await navigator.mediaDevices.getUserMedia({video:{facingMode:'user',width:{ideal:1280},height:{ideal:720}},audio:false});video.srcObject=stream;result.className='result';result.textContent='摄像头已启动，正在识别…';if(timer)clearInterval(timer);timer=setInterval(scan,2600);setTimeout(scan,900);}catch(error){result.className='result bad';result.textContent='无法打开摄像头：'+error.message;}
  });
  async function scan(){if(busy||!stream)return;busy=true;try{const blob=await captureBlob(video);const form=new FormData();form.append('file',blob,'capture.jpg');form.append('classroom',classroom.value.trim());const headers={};if(key.value)headers['X-Kiosk-Key']=key.value;const response=await api('/api/attendance/recognize',{method:'POST',body:form,headers});result.className='result ok';result.textContent=`${response.student.name}（${response.student.student_no}） · ${statusText[response.record.status]||response.record.status}`;await new Promise(r=>setTimeout(r,1800));}catch(error){result.className='result bad';result.textContent=error.message;}finally{busy=false;}}
}

boot();
