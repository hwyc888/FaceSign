let faceCompareBlob = null;
let faceComparePreviewURL = '';
let faceCompareStream = null;

function setFaceComparePhoto(blob) {
  faceCompareBlob = blob || null;
  const preview = $('#faceComparePreview');
  const wrap = $('#faceComparePreviewWrap');
  if (faceComparePreviewURL) {
    URL.revokeObjectURL(faceComparePreviewURL);
    faceComparePreviewURL = '';
  }
  if (faceCompareBlob) {
    faceComparePreviewURL = URL.createObjectURL(faceCompareBlob);
    preview.src = faceComparePreviewURL;
    wrap.classList.remove('hidden');
  } else {
    preview.removeAttribute('src');
    wrap.classList.add('hidden');
  }
  $('#faceCompareRun').disabled = !faceCompareBlob;
  $('#faceCompareFile').value = '';
  $('#faceCompareResult').classList.add('hidden');
  $('#faceCompareEmpty').classList.remove('hidden');
}

function resetFaceComparePage() {
  stopFaceCompareCamera();
  $('#faceCompareCapture').disabled = true;
  $('#faceCompareCloseCamera').disabled = true;
  $('#faceCompareOpenCamera').disabled = false;
}

$('#faceCompareFile').addEventListener('change', event => {
  const file = event.target.files && event.target.files[0];
  if (!file) return;
  if (!['image/jpeg', 'image/png'].includes(file.type)) {
    toast('请选择 JPG、PNG 或 WEBP 图片');
    event.target.value = '';
    return;
  }
  if (file.size > 8 * 1024 * 1024) {
    toast('图片不能超过 8 MB');
    event.target.value = '';
    return;
  }
  setFaceComparePhoto(file);
});

$('#faceCompareRemove').addEventListener('click', () => setFaceComparePhoto(null));

async function openFaceCompareCamera() {
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    toast('当前浏览器不支持摄像头访问，请使用 HTTPS 或本机地址');
    return;
  }
  try {
    faceCompareStream = await navigator.mediaDevices.getUserMedia({
      video: {facingMode: 'user', width: {ideal: 1280}, height: {ideal: 720}},
      audio: false
    });
    const video = $('#faceCompareCamera');
    video.srcObject = faceCompareStream;
    await video.play();
    $('#faceCompareCameraWrap').classList.remove('hidden');
    $('#faceCompareCapture').disabled = false;
    $('#faceCompareCloseCamera').disabled = false;
    $('#faceCompareOpenCamera').disabled = true;
  } catch (error) {
    stopFaceCompareCamera();
    toast('无法打开摄像头：' + (error.message || error));
  }
}

function stopFaceCompareCamera() {
  if (faceCompareStream) {
    faceCompareStream.getTracks().forEach(track => track.stop());
    faceCompareStream = null;
  }
  const video = $('#faceCompareCamera');
  if (video) video.srcObject = null;
  $('#faceCompareCameraWrap').classList.add('hidden');
}

async function captureFaceComparePhoto() {
  const video = $('#faceCompareCamera');
  if (!video || !video.videoWidth || !video.videoHeight) {
    toast('摄像头尚未准备好，请稍候');
    return;
  }
  const canvas = document.createElement('canvas');
  canvas.width = video.videoWidth;
  canvas.height = video.videoHeight;
  const context = canvas.getContext('2d');
  context.drawImage(video, 0, 0, canvas.width, canvas.height);
  const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/jpeg', 0.92));
  if (!blob) {
    toast('拍照失败，请重试');
    return;
  }
  setFaceComparePhoto(blob);
  toast('照片已拍摄，可以开始比对');
}

function renderFaceCompareResult(result) {
  const threshold = Number(result.threshold || 0);
  const best = result.best;
  $('#faceCompareEmpty').classList.add('hidden');
  $('#faceCompareResult').classList.remove('hidden');
  $('#faceCompareThreshold').textContent = (threshold * 100).toFixed(1) + '%';
  $('#faceCompareBestScore').textContent = best ? (Number(best.similarity) * 100).toFixed(1) + '%' : '--';
  const decision = $('#faceCompareDecision');
  decision.textContent = result.matched ? '达到当前阈值' : '未达到当前阈值';
  decision.classList.toggle('face-compare-positive', Boolean(result.matched));
  decision.classList.toggle('face-compare-negative', !result.matched);

  const bestMatch = $('#faceCompareBestMatch');
  if (!best) {
    bestMatch.innerHTML = '<p class="muted">当前人脸库暂无可用于比对的样本。</p>';
  } else {
    bestMatch.innerHTML = '<strong>' + esc(best.student.name) + '</strong>' +
      '<span>学号：' + esc(best.student.student_no) + '</span>' +
      '<span>班级：' + esc(best.student.class_name || '-') + '</span>' +
      '<span>该学生样本数：' + Number(best.sample_count || 0) + '</span>';
  }

  const candidates = Array.isArray(result.candidates) ? result.candidates : [];
  $('#faceCompareCandidates').innerHTML = candidates.map((item, index) => {
    const score = Math.max(0, Math.min(1, Number(item.similarity) || 0));
    return '<div class="face-compare-candidate">' +
      '<span class="face-compare-rank">' + (index + 1) + '</span>' +
      '<div class="face-compare-candidate-main"><strong>' + esc(item.student.name) + '</strong>' +
      '<small>' + esc(item.student.student_no) + ' · ' + esc(item.student.class_name || '-') + '</small>' +
      '<div class="face-compare-score-track"><i style="width:' + (score * 100).toFixed(1) + '%"></i></div></div>' +
      '<strong class="face-compare-candidate-score">' + (score * 100).toFixed(1) + '%</strong>' +
      '</div>';
  }).join('') || '<p class="muted">暂无可比较的人脸样本。</p>';
  $('#faceCompareModelInfo').textContent = '模型：' + (result.engine || 'ArcFace ResNet50') +
    ' · 版本：' + (result.model_version || '--') +
    ' · 人脸库学生：' + Number(result.library_students || 0) + ' 人';
}

$('#faceCompareOpenCamera').addEventListener('click', openFaceCompareCamera);
$('#faceCompareCapture').addEventListener('click', captureFaceComparePhoto);
$('#faceCompareCloseCamera').addEventListener('click', () => {
  stopFaceCompareCamera();
  $('#faceCompareCapture').disabled = true;
  $('#faceCompareCloseCamera').disabled = true;
  $('#faceCompareOpenCamera').disabled = false;
});

$('#faceCompareRun').addEventListener('click', async () => {
  if (!faceCompareBlob) return;
  const button = $('#faceCompareRun');
  button.disabled = true;
  button.textContent = '正在提取特征并比对…';
  try {
    const form = new FormData();
    form.append('file', faceCompareBlob, 'face-compare.jpg');
    const result = await api('/api/face-compare', {method: 'POST', body: form});
    renderFaceCompareResult(result);
  } catch (error) {
    toast(error.message || '人脸比对失败');
  } finally {
    button.disabled = !faceCompareBlob;
    button.textContent = '开始人脸库比对';
  }
});

window.addEventListener('beforeunload', () => {
  stopFaceCompareCamera();
  if (faceComparePreviewURL) URL.revokeObjectURL(faceComparePreviewURL);
});
