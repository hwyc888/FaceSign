const $ = s => document.querySelector(s);
const $$ = s => document.querySelectorAll(s);

let stream = null;
let autoTimer = null;
let recognizing = false;
let manualRecognitionBurst = false;
let recognitionSessionID = makeRecognitionSessionID();
let pendingEnrollmentBlob = null;
let studentsCache = [];
let classesCache = [];
let duplicateStudent = null;
let samplesStudent = null;
let supplementStudent = null;
let camerasCache = [];
let camerasLoaded = false;
let cameraOpen = false;
let activeCamera = null;

function makeRecognitionSessionID() {
  if (window.crypto && typeof window.crypto.randomUUID === 'function') {
    return window.crypto.randomUUID();
  }
  return 'cam-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2);
}

function resetRecognitionSession() {
  recognitionSessionID = makeRecognitionSessionID();
}

const titles = {
  checkin: ['人脸签到', '摄像头可同时识别多人并完成签到'],
  students: ['学生管理', '先拍人脸、自动查重，再录入学生信息；支持多角度样本'],
  seating: ['座位编排', '按班级管理学生座号，支持批量安排和拖动换位'],
  attendance: ['考勤记录', '每名学生每天首次识别记为签到'],
  settings: ['设置', '签到启动方式、摄像头管理、班级管理与基础数据设置'],
  users: ['人员管理', '创建普通账号，并分别设置学生资料、人脸采样和座位编排权限'],
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
    if (r.status === 401 &&
      !['/api/auth/status', '/api/auth/login', '/api/auth/setup', '/api/auth/logout'].includes(url) &&
      typeof handleAuthExpired === 'function') {
      handleAuthExpired();
    }
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


function decorateResponsiveTables(root = document) {
  root.querySelectorAll('table').forEach(table => {
    const labels = Array.from(table.querySelectorAll('thead th')).map(th => th.textContent.trim());
    if (!labels.length) return;
    table.querySelectorAll('tbody tr').forEach(row => {
      Array.from(row.children).forEach((cell, index) => {
        if (cell.tagName === 'TD' && !cell.dataset.label) {
          cell.dataset.label = labels[index] || '';
        }
      });
    });
  });
}

let responsiveTableDecorationQueued = false;
const responsiveTableObserver = new MutationObserver(mutations => {
  if (!mutations.some(mutation => mutation.type === 'childList')) return;
  if (responsiveTableDecorationQueued) return;
  responsiveTableDecorationQueued = true;
  queueMicrotask(() => {
    responsiveTableDecorationQueued = false;
    decorateResponsiveTables();
  });
});
responsiveTableObserver.observe(document.body, {childList: true, subtree: true});
decorateResponsiveTables();
