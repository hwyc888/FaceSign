function updateRecognitionCounts(r = {}) {
  const verified = Math.max(0, Number(r.verified_count ?? r.recognized_count ?? 0) || 0);
  const unregistered = Math.max(0, Number(r.unregistered_count ?? 0) || 0);
  const verifiedEl = $('#recognitionVerifiedCount');
  const unregisteredEl = $('#recognitionUnregisteredCount');
  if (verifiedEl) verifiedEl.textContent = String(verified);
  if (unregisteredEl) unregisteredEl.textContent = String(unregistered);
}

function renderRecognition(r) {
  updateRecognitionCounts(r);
  const faces = Array.isArray(r.faces) ? r.faces : [];
  const persons = Array.isArray(r.persons) ? r.persons : [];
  const el = $('#result');
  el.className = 'result multi';

  if (!faces.length && !persons.length) {
    el.className = 'result empty';
    el.textContent = '未检测到人员或人脸';
    drawFaceOverlay([], []);
    return;
  }

  const summary = `人体跟踪 ${r.tracked_person_count || persons.length} 人，检测人脸 ${r.detected_count || faces.length} 张，等待露脸/清晰 ${r.waiting_face_count || 0} 人，签到通过 ${r.verified_count || 0} 人，验证中 ${r.pending_count || 0} 人，需重新对准 ${r.timeout_count || 0} 人，疑似照片/屏幕 ${r.spoof_count || 0} 人，未录入 ${r.unregistered_count || 0} 人`;

  const faceHTML = faces.map((f, i) => {
    const quality = Number(f.quality_score || 0);
    const qualityText = `人脸质量 ${(quality * 100).toFixed(0)}% · ${esc(f.quality_status || '等待清晰')}`;
    if (f.recognized && f.student) {
      const attendanceMeta = f.attendance
        ? ` | 首次 ${esc(String(f.attendance.checked_at || '').slice(-8))} | 最近 ${esc(String(f.attendance.last_seen_at || f.attendance.checked_at || '').slice(-8))} | 今日 ${Number(f.attendance.recognition_count || 1)}次`
        : '';
      return `<div class="face-result known">
        <div class="face-title">${esc(f.student.name)} · ${f.first_checkin_today ? '签到成功' : '今日已签到'}</div>
        <div class="face-meta">${esc(f.student.student_no)} | ${esc(f.student.class_name || '-')} | 身份 ${(f.similarity * 100).toFixed(1)}% | 活体 ${(Number(f.liveness_score || 0) * 100).toFixed(1)}% | ${qualityText}${attendanceMeta}</div>
      </div>`;
    }
    if (f.matched && f.student && f.status === '疑似照片/屏幕') {
      return `<div class="face-result danger">
        <div class="face-title">${esc(f.student.name)} · 疑似照片/屏幕，拒绝签到</div>
        <div class="face-meta">身份 ${(f.similarity * 100).toFixed(1)}% | 活体 ${(Number(f.liveness_score || 0) * 100).toFixed(1)}% | ${qualityText} | 不写入签到记录</div>
      </div>`;
    }
    if (f.matched && f.student && quality < Number(r.face_quality_threshold || 0.45)) {
      return `<div class="face-result pending warning">
        <div class="face-title">${esc(f.student.name)} · 等待更清晰人脸</div>
        <div class="face-meta">已由人体 Track 保持身份 | ${qualityText} | 最佳 ${(Number(f.best_quality || 0) * 100).toFixed(0)}%</div>
      </div>`;
    }
    if (f.matched && f.student) {
      const timeoutClass = f.liveness_timed_out ? ' warning' : '';
      return `<div class="face-result pending${timeoutClass}">
        <div class="face-title">${esc(f.student.name)} · ${esc(f.status || '活体验证中')}</div>
        <div class="face-meta">身份 ${(f.similarity * 100).toFixed(1)}% | 活体 ${(Number(f.liveness_score || 0) * 100).toFixed(1)}% | ${qualityText} | 连续帧 ${f.liveness_frames || 0}/${f.required_frames || r.liveness_fast_frames || 3}</div>
      </div>`;
    }
    if (quality < Number(r.face_quality_threshold || 0.45)) {
      return `<div class="face-result pending warning">
        <div class="face-title">等待清晰人脸</div>
        <div class="face-meta">Track ${esc(f.track_id || '-')} | ${qualityText} | 系统会继续跟踪并选择后续最佳帧</div>
      </div>`;
    }
    return `<div class="face-result unknown">
      <div class="face-title">未录入</div>
      <div class="face-meta">人脸 ${i + 1} 未匹配到已录入学生 | ${qualityText}</div>
    </div>`;
  }).join('');

  const waitingPersons = persons.filter(p => !p.face_visible);
  const waitingHTML = waitingPersons.slice(0, 4).map(p => `
    <div class="face-result pending">
      <div class="face-title">${p.student ? esc(p.student.name) + ' · ' : ''}人体 Track ${esc(p.track_id || '-')} · 等待露脸</div>
      <div class="face-meta">已持续跟踪该人员；出现清晰正脸后自动选择最佳帧识别${p.student ? ` | 已缓存身份 ${(Number(p.similarity || 0) * 100).toFixed(1)}%` : ''}</div>
    </div>
  `).join('');

  el.innerHTML = `<div class="result-summary">${summary}</div>` + faceHTML + waitingHTML;
  drawFaceOverlay(faces, persons);
}

function drawFaceOverlay(faces, persons = []) {
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

  persons.forEach(p => {
    const b = p.box || {};
    if (p.face_visible) return;
    ctx.save();
    ctx.strokeStyle = '#8b5cf6';
    ctx.fillStyle = '#8b5cf6';
    ctx.setLineDash([10, 8]);
    ctx.strokeRect(b.x || 0, b.y || 0, b.width || 0, b.height || 0);
    const label = p.student ? p.student.name + ' · 等待露脸' : '人员 · 等待露脸';
    ctx.fillText(label, Math.max(0, b.x || 0), Math.max(24, b.y || 0) - 5);
    ctx.restore();
  });

  faces.forEach(f => {
    const b = f.box || {};
    let color = '#f59e0b';
    let label = '未录入';
    const quality = Math.round(Number(f.quality_score || 0) * 100);
    if (f.recognized && f.student) {
      color = '#22c55e';
      label = f.student.name + ' · 已签到';
    } else if (f.matched && f.status === '疑似照片/屏幕') {
      color = '#ef4444';
      label = (f.student ? f.student.name + ' · ' : '') + '疑似攻击';
    } else if (Number(f.quality_score || 0) < 0.45) {
      color = '#f59e0b';
      label = (f.student ? f.student.name + ' · ' : '') + `等待清晰 ${quality}%`;
    } else if (f.matched && f.liveness_timed_out) {
      color = '#f59e0b';
      label = (f.student ? f.student.name + ' · ' : '') + '请重新对准';
    } else if (f.matched && f.student) {
      color = '#3b82f6';
      label = f.student.name + ' · 活体验证中';
    }
    ctx.strokeStyle = color;
    ctx.fillStyle = color;
    ctx.setLineDash([]);
    ctx.strokeRect(b.x || 0, b.y || 0, b.width || 0, b.height || 0);
    const x = Math.max(0, b.x || 0);
    const y = Math.max(24, b.y || 0);
    ctx.fillText(label, x, y - 5);
  });
}

async function recognizeFrame(options = {}) {
  if (recognizing) return null;
  recognizing = true;
  const performanceStartedAt = performance.now();
  try {
    let r;
    if (activeCamera && activeCamera.kind !== 'local') {
      r = await api(`/api/cameras/${activeCamera.id}/recognize`, {
        method: 'POST',
        headers: {'X-FaceSign-Session': recognitionSessionID}
      });
    } else {
      const blob = await capture();
      const fd = new FormData();
      fd.append('file', blob, 'camera.jpg');
      r = await api('/api/recognize', {
        method: 'POST',
        headers: {'X-FaceSign-Session': recognitionSessionID},
        body: fd
      });
    }
    renderRecognition(r);
    if ((r.verified_count || r.recognized_count || 0) > 0) {
      loadToday();
      if (typeof loadCheckinSeatBoard === 'function') loadCheckinSeatBoard();
    }
    return r;
  } catch (e) {
    updateRecognitionCounts({});
    drawFaceOverlay([], []);
    if (!autoTimer && !options.silent) toast(e.message);
    return null;
  } finally {
    if (typeof recordRecognitionRealtimeSample === 'function') {
      recordRecognitionRealtimeSample(performance.now() - performanceStartedAt);
    }
    recognizing = false;
  }
}

function recognitionFrameIntervalMS() {
  if (activeCamera && activeCamera.kind !== 'local') {
    const fps = Math.max(1, Math.min(Number(activeCamera.fps || 5), 5));
    return Math.max(200, Math.round(1000 / fps));
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
      const qualityThreshold = Number(result.face_quality_threshold || 0.45);
      const terminalFaces = faces.length && faces.every(face =>
        face.recognized ||
        face.status === '疑似照片/屏幕' ||
        face.liveness_timed_out ||
        (!face.matched && Number(face.quality_score || 0) >= qualityThreshold)
      );
      const waitingPerson = Array.isArray(result.persons) && result.persons.some(person =>
        !person.face_visible || Number(person.face_quality || 0) < qualityThreshold
      );
      if (terminalFaces && !waitingPerson) {
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
