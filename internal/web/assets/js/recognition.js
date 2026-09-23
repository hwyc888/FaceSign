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
    `<div class="result-summary">检测 ${r.detected_count} 人，签到通过 ${r.verified_count || 0} 人，验证中 ${r.pending_count || 0} 人，需重新对准 ${r.timeout_count || 0} 人，疑似照片/屏幕 ${r.spoof_count || 0} 人，未录入 ${r.unregistered_count || 0} 人</div>` +
    faces.map((f, i) => {
      if (f.recognized && f.student) {
        const attendanceMeta = f.attendance
          ? ` | 首次 ${esc(String(f.attendance.checked_at || '').slice(-8))} | 最近 ${esc(String(f.attendance.last_seen_at || f.attendance.checked_at || '').slice(-8))} | 今日 ${Number(f.attendance.recognition_count || 1)}次`
          : '';
        return `<div class="face-result known">
          <div class="face-title">${esc(f.student.name)} · ${f.first_checkin_today ? '签到成功' : '今日已签到'}</div>
          <div class="face-meta">${esc(f.student.student_no)} | ${esc(f.student.class_name || '-')} | 身份 ${(f.similarity * 100).toFixed(1)}% | 活体 ${(Number(f.liveness_score || 0) * 100).toFixed(1)}%${attendanceMeta}</div>
        </div>`;
      }
      if (f.matched && f.student && f.status === '疑似照片/屏幕') {
        return `<div class="face-result danger">
          <div class="face-title">${esc(f.student.name)} · 疑似照片/屏幕，拒绝签到</div>
          <div class="face-meta">身份 ${(f.similarity * 100).toFixed(1)}% | 活体 ${(Number(f.liveness_score || 0) * 100).toFixed(1)}% | 不写入签到记录</div>
        </div>`;
      }
      if (f.matched && f.student) {
        const timeoutClass = f.liveness_timed_out ? ' warning' : '';
        return `<div class="face-result pending${timeoutClass}">
          <div class="face-title">${esc(f.student.name)} · ${esc(f.status || '活体验证中')}</div>
          <div class="face-meta">身份 ${(f.similarity * 100).toFixed(1)}% | 活体 ${(Number(f.liveness_score || 0) * 100).toFixed(1)}% | 连续帧 ${f.liveness_frames || 0}/${f.required_frames || r.liveness_fast_frames || 3}</div>
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
  const overlay = $('#faceOverlay');
  if (!overlay) return;
  const dimensions = typeof cameraFrameDimensions === 'function'
    ? cameraFrameDimensions('#camera')
    : {width: 1280, height: 720};
  overlay.width = dimensions.width || 1280;
  overlay.height = dimensions.height || 720;
  const ctx = overlay.getContext('2d');
  ctx.clearRect(0, 0, overlay.width, overlay.height);
  ctx.lineWidth = Math.max(2, overlay.width / 400);
  ctx.font = `${Math.max(18, Math.round(overlay.width / 55))}px Segoe UI, Arial`;
  faces.forEach(f => {
    const b = f.box || {};
    let color = '#f59e0b';
    let label = '未录入';
    if (f.recognized && f.student) {
      color = '#22c55e';
      label = f.student.name + ' · 已签到';
    } else if (f.matched && f.status === '疑似照片/屏幕') {
      color = '#ef4444';
      label = (f.student ? f.student.name + ' · ' : '') + '疑似攻击';
    } else if (f.matched && f.liveness_timed_out) {
      color = '#f59e0b';
      label = (f.student ? f.student.name + ' · ' : '') + '请重新对准';
    } else if (f.matched && f.student) {
      color = '#3b82f6';
      label = f.student.name + ' · 活体验证中';
    }
    ctx.strokeStyle = color;
    ctx.fillStyle = color;
    ctx.strokeRect(b.x || 0, b.y || 0, b.width || 0, b.height || 0);
    const x = Math.max(0, b.x || 0);
    const y = Math.max(24, b.y || 0);
    ctx.fillText(label, x, y - 5);
  });
}

async function recognizeFrame(options = {}) {
  if (recognizing) return null;
  recognizing = true;
  try {
    const blob = await capture();
    const fd = new FormData();
    fd.append('file', blob, 'camera.jpg');
    const r = await api('/api/recognize', {
      method: 'POST',
      headers: {'X-FaceSign-Session': recognitionSessionID},
      body: fd
    });
    renderRecognition(r);
    if ((r.verified_count || r.recognized_count || 0) > 0) {
      loadToday();
      if (typeof loadCheckinSeatBoard === 'function') loadCheckinSeatBoard();
    }
    return r;
  } catch (e) {
    drawFaceOverlay([]);
    if (!autoTimer && !options.silent) toast(e.message);
    return null;
  } finally {
    recognizing = false;
  }
}

function recognitionFrameIntervalMS() {
  if (activeCamera && activeCamera.kind !== 'local') {
    const fps = Math.max(1, Math.min(Number(activeCamera.fps || 5), 12));
    return Math.max(120, Math.round(1000 / fps));
  }
  return 120;
}

async function recognizeBurst() {
  if (manualRecognitionBurst) return;
  manualRecognitionBurst = true;
  const startedAt = performance.now();
  let maxFrames = 6;
  try {
    for (let i = 0; i < maxFrames && performance.now() - startedAt < 2100; i++) {
      const result = await recognizeFrame({silent: true});
      if (!result) break;
      maxFrames = Math.max(3, Number(result.liveness_max_frames || maxFrames));
      const faces = Array.isArray(result.faces) ? result.faces : [];
      if (faces.length && faces.every(face =>
        face.recognized ||
        face.status === '疑似照片/屏幕' ||
        face.liveness_timed_out ||
        !face.matched
      )) {
        break;
      }
      if (i + 1 < maxFrames && performance.now() - startedAt < 1980) {
        await new Promise(resolve => setTimeout(resolve, recognitionFrameIntervalMS()));
      }
    }
  } finally {
    manualRecognitionBurst = false;
  }
}

async function runAutoRecognitionLoop() {
  const checkinActive = $('#page-checkin')?.classList.contains('active');
  if (!$('#autoScan')?.checked || !cameraOpen || !checkinActive) {
    autoTimer = null;
    return;
  }
  await recognizeFrame({silent: true});
  if ($('#autoScan')?.checked && cameraOpen && $('#page-checkin')?.classList.contains('active')) {
    autoTimer = setTimeout(runAutoRecognitionLoop, recognitionFrameIntervalMS());
  } else {
    autoTimer = null;
  }
}

function stopAutoRecognition() {
  clearTimeout(autoTimer);
  autoTimer = null;
  const checkbox = $('#autoScan');
  if (checkbox) checkbox.checked = false;
}

async function setAutoRecognitionEnabled(enabled) {
  stopAutoRecognition();
  if (!enabled) return;
  const checkbox = $('#autoScan');
  if (!checkbox) return;
  checkbox.checked = true;
  try {
    await startCamera();
    if (checkbox.checked && $('#page-checkin')?.classList.contains('active')) {
      autoTimer = setTimeout(runAutoRecognitionLoop, 0);
    }
  } catch (e) {
    checkbox.checked = false;
    throw e;
  }
}

$('#startCamera').addEventListener('click', toggleCamera);
$('#recognize').addEventListener('click', recognizeBurst);
$('#autoScan').addEventListener('change', async e => {
  try {
    await setAutoRecognitionEnabled(e.target.checked);
  } catch {
    // startCamera already shows the camera/HTTPS error.
  }
});
