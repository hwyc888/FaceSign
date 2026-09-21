const $ = s => document.querySelector(s);
const $$ = s => document.querySelectorAll(s);

let stream = null;
let autoTimer = null;
let recognizing = false;
let pendingEnrollmentBlob = null;
let studentsCache = [];
let duplicateStudent = null;
let samplesStudent = null;
let supplementStudent = null;

const titles = {
  checkin: ['人脸签到', '摄像头可同时识别多人并完成签到'],
  students: ['学生管理', '先拍人脸、自动查重，再录入学生信息；支持多角度样本'],
  attendance: ['考勤记录', '每名学生每天首次识别记为签到'],
  status: ['系统状态', '检查人脸引擎、数据库和部署状态']
};

function toast(msg) {
  const el = $('#toast');
  el.textContent = msg;
  el.classList.add('show');
  setTimeout(() => el.classList.remove('show'), 2800);
}

async function api(url, opt = {}) {
  const r = await fetch(url, opt);
  const data = await r.json().catch(() => ({}));
  if (!r.ok) {
    const err = new Error(data.error || `HTTP ${r.status}`);
    err.status = r.status;
    err.data = data;
    throw err;
  }
  return data;
}

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

function updateCameraControls() {
  const opened = !!stream;
  const checkinButton = $('#startCamera');
  const enrollButton = $('#toggleEnrollCamera');
  if (checkinButton) checkinButton.textContent = opened ? '关闭摄像头' : '打开摄像头';
  if (enrollButton) enrollButton.textContent = opened ? '关闭摄像头' : '打开摄像头';
  if ($('#recognize')) $('#recognize').disabled = !opened;
  if ($('#captureEnrollment')) $('#captureEnrollment').disabled = !opened;
}

async function startCamera() {
  if (stream) {
    await attachCameraViews();
    updateCameraControls();
    return;
  }
  try {
    stream = await navigator.mediaDevices.getUserMedia({
      video: {width: {ideal: 1280}, height: {ideal: 720}, facingMode: 'user'},
      audio: false
    });
    await attachCameraViews();
    updateCameraControls();
    toast('摄像头已打开');
  } catch (e) {
    stream = null;
    updateCameraControls();
    toast('无法打开摄像头：' + e.message);
    throw e;
  }
}

function stopCamera() {
  if (stream) {
    stream.getTracks().forEach(track => track.stop());
    stream = null;
  }
  ['#camera', '#enrollCamera'].map($).filter(Boolean).forEach(video => {
    video.srcObject = null;
  });
  if (autoTimer) {
    clearInterval(autoTimer);
    autoTimer = null;
  }
  if ($('#autoScan')) $('#autoScan').checked = false;
  drawFaceOverlay([]);
  updateCameraControls();
  toast('摄像头已关闭');
}

async function toggleCamera() {
  if (stream) stopCamera();
  else await startCamera();
}

async function attachCameraViews() {
  const views = ['#camera', '#enrollCamera'].map($).filter(Boolean);
  for (const video of views) {
    if (video.srcObject !== stream) video.srcObject = stream;
    if (video.readyState < 1) {
      await new Promise(resolve => video.addEventListener('loadedmetadata', resolve, {once: true}));
    }
    await video.play();
  }
}

async function capture(selector = '#camera') {
  if (!stream) throw new Error('请先打开摄像头');
  const video = $(selector);
  const canvas = $('#canvas');
  if (!video || !video.videoWidth || !video.videoHeight) {
    throw new Error('摄像头画面尚未准备好');
  }
  canvas.width = video.videoWidth;
  canvas.height = video.videoHeight;
  canvas.getContext('2d').drawImage(video, 0, 0, canvas.width, canvas.height);
  return await new Promise((resolve, reject) => {
    canvas.toBlob(
      blob => blob ? resolve(blob) : reject(new Error('拍照失败')),
      'image/jpeg',
      0.92
    );
  });
}

function renderRecognition(r) {
  const faces = Array.isArray(r.faces) ? r.faces : [];
  const el = $('#result');
  el.className = 'result multi';
  if (!faces.length) {
    el.className = 'result empty';
    el.textContent = '未检测到人脸';
    drawFaceOverlay([]);
    return;
  }
  el.innerHTML =
    `<div class="result-summary">检测 ${r.detected_count} 人，已录入 ${r.recognized_count} 人，未录入 ${r.unregistered_count} 人</div>` +
    faces.map((f, i) => {
      if (f.recognized && f.student) {
        return `<div class="face-result known">
          <div class="face-title">${esc(f.student.name)} · ${f.first_checkin_today ? '签到成功' : '今日已签到'}</div>
          <div class="face-meta">${esc(f.student.student_no)} | ${esc(f.student.class_name || '-')} | 相似度 ${(f.similarity * 100).toFixed(1)}%</div>
        </div>`;
      }
      return `<div class="face-result unknown">
        <div class="face-title">未录入</div>
        <div class="face-meta">人脸 ${i + 1} 未匹配到已录入学生</div>
      </div>`;
    }).join('');
  drawFaceOverlay(faces);
}

function drawFaceOverlay(faces) {
  const video = $('#camera');
  const overlay = $('#faceOverlay');
  if (!overlay || !video) return;
  overlay.width = video.videoWidth || 1280;
  overlay.height = video.videoHeight || 720;
  const ctx = overlay.getContext('2d');
  ctx.clearRect(0, 0, overlay.width, overlay.height);
  ctx.lineWidth = Math.max(2, overlay.width / 400);
  ctx.font = `${Math.max(18, Math.round(overlay.width / 55))}px Segoe UI, Arial`;
  faces.forEach(f => {
    const b = f.box || {};
    const label = f.recognized && f.student ? f.student.name : '未录入';
    ctx.strokeStyle = f.recognized ? '#22c55e' : '#f59e0b';
    ctx.fillStyle = ctx.strokeStyle;
    ctx.strokeRect(b.x || 0, b.y || 0, b.width || 0, b.height || 0);
    const x = Math.max(0, b.x || 0);
    const y = Math.max(24, b.y || 0);
    ctx.fillText(label, x, y - 5);
  });
}

async function recognize() {
  if (recognizing) return;
  recognizing = true;
  try {
    const blob = await capture();
    const fd = new FormData();
    fd.append('file', blob, 'camera.jpg');
    const r = await api('/api/recognize', {method: 'POST', body: fd});
    renderRecognition(r);
    if (r.recognized_count > 0) loadToday();
  } catch (e) {
    drawFaceOverlay([]);
    if (!autoTimer) toast(e.message);
  } finally {
    recognizing = false;
  }
}

$('#startCamera').addEventListener('click', toggleCamera);
$('#recognize').addEventListener('click', recognize);
$('#autoScan').addEventListener('change', async e => {
  clearInterval(autoTimer);
  autoTimer = null;
  if (e.target.checked) {
    try {
      await startCamera();
      autoTimer = setInterval(recognize, 2500);
    } catch {
      e.target.checked = false;
    }
  }
});

function openModal(id) {
  $('#' + id).classList.remove('hidden');
}

function closeModal(id) {
  $('#' + id).classList.add('hidden');
}

$$('[data-close]').forEach(button => {
  button.addEventListener('click', () => closeModal(button.dataset.close));
});

function showDuplicate(data) {
  duplicateStudent = data.student || null;
  if (!duplicateStudent) {
    toast(data.error || '该人脸已经录入');
    return;
  }
  const similarity = Number(data.similarity || 0);
  $('#duplicateInfo').innerHTML = `
    <div class="duplicate-name">${esc(duplicateStudent.name)}</div>
    <div>学号：${esc(duplicateStudent.student_no)}</div>
    <div>班级：${esc(duplicateStudent.class_name || '-')}</div>
    <div>现有人脸样本：${duplicateStudent.face_count || 1} 个</div>
    <div>本次相似度：${(similarity * 100).toFixed(1)}%</div>
  `;
  openModal('duplicateModal');
}

$('#duplicateViewSamples').addEventListener('click', async () => {
  if (!duplicateStudent) return;
  closeModal('duplicateModal');
  await openSamplesModal(duplicateStudent.id);
});

async function captureEnrollment() {
  try {
    await startCamera();
    const blob = await capture('#enrollCamera');

    if (supplementStudent) {
      const fd = new FormData();
      fd.append('file', blob, 'face.jpg');
      fd.append('label', $('#supplementAngle').value);
      try {
        await api(`/api/students/${supplementStudent.id}/faces`, {method: 'POST', body: fd});
      } catch (e) {
        if (e.data && e.data.duplicate) {
          showDuplicate(e.data);
          return;
        }
        throw e;
      }
      const studentID = supplementStudent.id;
      toast('多角度人脸样本已补充');
      cancelSupplementMode();
      await loadStudents();
      await openSamplesModal(studentID);
      return;
    }

    const checkForm = new FormData();
    checkForm.append('file', blob, 'face.jpg');
    const check = await api('/api/enrollment/check', {method: 'POST', body: checkForm});
    if (check.duplicate) {
      showDuplicate(check);
      return;
    }

    pendingEnrollmentBlob = blob;
    $('#studentInfoForm').reset();
    openModal('studentModal');
    setTimeout(() => $('#modalStudentNo').focus(), 50);
  } catch (e) {
    toast(e.message);
  }
}

$('#toggleEnrollCamera').addEventListener('click', toggleCamera);
$('#captureEnrollment').addEventListener('click', captureEnrollment);

$('#studentInfoForm').addEventListener('submit', async e => {
  e.preventDefault();
  if (!pendingEnrollmentBlob) {
    toast('请重新拍摄人脸');
    closeModal('studentModal');
    return;
  }

  const fd = new FormData();
  fd.append('file', pendingEnrollmentBlob, 'face.jpg');
  fd.append('student_no', $('#modalStudentNo').value);
  fd.append('name', $('#modalStudentName').value);
  fd.append('class_name', $('#modalClassName').value);
  fd.append('label', '正面');

  try {
    await api('/api/enrollment/create', {method: 'POST', body: fd});
    pendingEnrollmentBlob = null;
    closeModal('studentModal');
    toast('学生信息和人脸已录入');
    await loadStudents();
    await loadHealth();
  } catch (e2) {
    if (e2.data && e2.data.duplicate) {
      closeModal('studentModal');
      pendingEnrollmentBlob = null;
      showDuplicate(e2.data);
      return;
    }
    toast(e2.message);
  }
});

async function loadStudents() {
  try {
    const items = await api('/api/students');
    studentsCache = items;
    $('#studentsBody').innerHTML = items.map(s => `
      <tr>
        <td>${esc(s.student_no)}</td>
        <td>${esc(s.name)}</td>
        <td>${esc(s.class_name || '-')}</td>
        <td><span class="badge ${s.face_count > 0 ? 'yes' : 'no'}">${s.face_count > 0 ? s.face_count + ' 个样本' : '未录入'}</span></td>
        <td>
          <div class="student-actions">
            <button data-faces="${s.id}">人脸样本</button>
            <button class="danger" data-del="${s.id}">删除</button>
          </div>
        </td>
      </tr>
    `).join('') || '<tr><td colspan="5">暂无学生</td></tr>';

    $$('[data-faces]').forEach(button => {
      button.onclick = () => openSamplesModal(Number(button.dataset.faces));
    });
    $$('[data-del]').forEach(button => {
      button.onclick = async () => {
        if (!confirm('确定删除该学生及其全部人脸样本、考勤数据？')) return;
        try {
          await api(`/api/students/${button.dataset.del}`, {method: 'DELETE'});
          await loadStudents();
          await loadHealth();
        } catch (e) {
          toast(e.message);
        }
      };
    });
  } catch (e) {
    toast(e.message);
  }
}

async function openSamplesModal(studentID) {
  const student = studentsCache.find(s => Number(s.id) === Number(studentID)) ||
    (duplicateStudent && Number(duplicateStudent.id) === Number(studentID) ? duplicateStudent : null);
  if (!student) {
    await loadStudents();
    return openSamplesModal(studentID);
  }

  samplesStudent = student;
  $('#samplesTitle').textContent = `${student.name} · 人脸样本`;
  $('#samplesHint').textContent = `${student.student_no} | ${student.class_name || '-'} · 可查询、删除或继续补充多角度样本`;
  openModal('samplesModal');

  try {
    const samples = await api(`/api/students/${student.id}/faces`);
    $('#samplesBody').innerHTML = samples.map(sample => `
      <tr>
        <td>${esc(sample.label || '补充')}</td>
        <td>${esc(sample.created_at)}</td>
        <td><button class="danger" data-delete-sample="${sample.id}">删除样本</button></td>
      </tr>
    `).join('') || '<tr><td colspan="3">该学生还没有人脸样本</td></tr>';

    $$('[data-delete-sample]').forEach(button => {
      button.onclick = async () => {
        if (!confirm('确定删除这个人脸样本？')) return;
        try {
          await api(`/api/students/${student.id}/faces/${button.dataset.deleteSample}`, {method: 'DELETE'});
          await loadStudents();
          await openSamplesModal(student.id);
        } catch (e) {
          toast(e.message);
        }
      };
    });
  } catch (e) {
    toast(e.message);
  }
}

$('#startSupplement').addEventListener('click', async () => {
  if (!samplesStudent) return;
  supplementStudent = samplesStudent;
  $('#supplementAngle').value = $('#sampleAngle').value;
  closeModal('samplesModal');
  $('#enrollMode').textContent = `补充：${supplementStudent.name}`;
  $('#supplementAngleWrap').classList.remove('hidden');
  $('#cancelSupplement').classList.remove('hidden');
  $('#captureEnrollment').textContent = '拍照补充';
  $('#enrollInstruction').innerHTML = `
    <strong>正在补充 ${esc(supplementStudent.name)} 的人脸样本</strong>
    <span>请选择角度，并确保镜头前只有该学生。</span>
    <span>系统会先与该学生已有样本做身份连续性校验，同时检查是否误录成其他学生。</span>
    <span>过于相似的重复角度会被拒绝，请适当改变朝向。</span>
  `;
  await startCamera();
});

function cancelSupplementMode() {
  supplementStudent = null;
  $('#enrollMode').textContent = '首次录入';
  $('#supplementAngleWrap').classList.add('hidden');
  $('#cancelSupplement').classList.add('hidden');
  $('#captureEnrollment').textContent = '拍照并录入';
  $('#enrollInstruction').innerHTML = `
    <strong>录入步骤</strong>
    <span>1. 正对摄像头并让脸部位于引导框内。</span>
    <span>2. 点击“拍照并录入”，系统先检查是否已经录入。</span>
    <span>3. 确认新面孔后，再填写学号、姓名和班级。</span>
  `;
}

$('#cancelSupplement').addEventListener('click', cancelSupplementMode);
$('#refreshStudents').addEventListener('click', loadStudents);

function today() {
  const d = new Date();
  const p = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

async function loadAttendance() {
  const day = $('#attendanceDay').value || today();
  $('#attendanceDay').value = day;
  try {
    const items = await api(`/api/attendance?day=${encodeURIComponent(day)}`);
    $('#attendanceBody').innerHTML = items.map(a => `
      <tr>
        <td>${esc(a.checked_at)}</td>
        <td>${esc(a.student_no)}</td>
        <td>${esc(a.name)}</td>
        <td>${esc(a.class_name || '-')}</td>
        <td>${(a.similarity * 100).toFixed(1)}%</td>
      </tr>
    `).join('') || '<tr><td colspan="5">暂无记录</td></tr>';
    $('#todaySummary').textContent = day === today() ? `今日已签到 ${items.length} 人` : '';
  } catch (e) {
    toast(e.message);
  }
}

function loadToday() {
  const old = $('#attendanceDay').value;
  $('#attendanceDay').value = today();
  loadAttendance().finally(() => {
    $('#attendanceDay').value = old || today();
  });
}

$('#refreshAttendance').addEventListener('click', loadAttendance);

function esc(v) {
  return String(v ?? '').replace(/[&<>"']/g, c => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  }[c]));
}

$('#attendanceDay').value = today();
updateCameraControls();
loadHealth();
loadStudents();
loadAttendance();
