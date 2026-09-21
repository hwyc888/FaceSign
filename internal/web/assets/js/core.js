const $ = s => document.querySelector(s);
const $$ = s => document.querySelectorAll(s);

let stream = null;
let autoTimer = null;
let recognizing = false;
let pendingEnrollmentBlob = null;
let studentsCache = [];
let classesCache = [];
let duplicateStudent = null;
let samplesStudent = null;
let supplementStudent = null;

const titles = {
  checkin: ['人脸签到', '摄像头可同时识别多人并完成签到'],
  students: ['学生管理', '先拍人脸、自动查重，再录入学生信息；支持多角度样本'],
  attendance: ['考勤记录', '每名学生每天首次识别记为签到'],
  settings: ['设置', '班级编排与基础数据设置'],
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

function openModal(id) {
  $('#' + id).classList.remove('hidden');
}

function closeModal(id) {
  $('#' + id).classList.add('hidden');
}

$$('[data-close]').forEach(button => {
  button.addEventListener('click', () => closeModal(button.dataset.close));
});

function esc(v) {
  return String(v ?? '').replace(/[&<>"']/g, c => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  }[c]));
}
