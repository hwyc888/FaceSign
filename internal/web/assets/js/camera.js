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
    const liveState = $('#enrollLiveState');
    if (liveState) {
      liveState.textContent = '摄像头已打开';
      liveState.className = 'capture-state ready';
    }
    const faceHint = $('#enrollFaceHint');
    if (faceHint) faceHint.textContent = '请将脸部完整放入引导框';
    if (typeof setEnrollmentStatus === 'function') {
      setEnrollmentStatus(
        supplementStudent ? '准备补充样本' : '摄像头已就绪',
        supplementStudent ? '调整到右侧选择的角度后拍照。' : '请正对摄像头，保持单人入镜。',
        'neutral'
      );
    }
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
  const liveState = $('#enrollLiveState');
  if (liveState) {
    liveState.textContent = '摄像头已关闭';
    liveState.className = 'capture-state neutral';
  }
  if (typeof setEnrollmentStatus === 'function') {
    setEnrollmentStatus('摄像头已关闭', '点击“打开摄像头”后再进行人脸采集。', 'neutral');
  }
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
