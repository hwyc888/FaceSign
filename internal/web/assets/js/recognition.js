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
    if (r.recognized_count > 0) {
      loadToday();
      if (typeof loadCheckinSeatBoard === 'function') loadCheckinSeatBoard();
    }
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
