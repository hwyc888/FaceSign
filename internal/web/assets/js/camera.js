function updateCameraControls() {
  const opened = cameraOpen;
  const checkinButton = $('#startCamera');
  const enrollButton = $('#toggleEnrollCamera');
  if (checkinButton) checkinButton.textContent = opened ? '关闭摄像头' : '打开摄像头';
  if (enrollButton) enrollButton.textContent = opened ? '关闭摄像头' : '打开摄像头';
  if ($('#recognize')) $('#recognize').disabled = !opened;
  if ($('#captureEnrollment')) $('#captureEnrollment').disabled = !opened;

  const configuredDefault = camerasCache.find(camera => camera.is_default) || null;
  const selected = activeCamera || configuredDefault;
  const label = selected ? selected.name : '浏览器默认摄像头';
  if ($('#checkinCameraName')) $('#checkinCameraName').textContent = label;
  if ($('#enrollCameraName')) $('#enrollCameraName').textContent = label;
}

async function loadCameraConfigs(force = false) {
  if (camerasLoaded && !force) return camerasCache;
  camerasCache = await api('/api/cameras');
  camerasLoaded = true;
  updateCameraControls();
  return camerasCache;
}

async function preferredCamera() {
  try {
    await loadCameraConfigs();
  } catch (e) {
    console.warn('load camera settings failed', e);
  }
  return camerasCache.find(camera => camera.is_default) || {
    id: 0,
    name: '浏览器默认摄像头',
    kind: 'local',
    device_id: '',
    protocol: 'browser',
    width: 1280,
    height: 720,
    fps: 30
  };
}

function switchCameraViews(network) {
  ['#camera', '#enrollCamera'].map($).filter(Boolean).forEach(video => {
    video.classList.toggle('hidden', network);
  });
  ['#cameraNetwork', '#enrollCameraNetwork'].map($).filter(Boolean).forEach(image => {
    image.classList.toggle('hidden', !network);
  });
}

async function fetchCameraFrameBlob(cameraID) {
  const response = await fetch(`/api/cameras/${cameraID}/frame?t=${Date.now()}`, {cache: 'no-store'});
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.error || `摄像头返回 HTTP ${response.status}`);
  }
  return await response.blob();
}

function activeNetworkCameraImage() {
  if ($('#page-students')?.classList.contains('active')) return $('#enrollCameraNetwork');
  return $('#cameraNetwork');
}

function stopNetworkPreview() {
  ['#cameraNetwork', '#enrollCameraNetwork'].map($).filter(Boolean).forEach(image => {
    image.removeAttribute('src');
  });
}

function startNetworkPreview() {
  if (!cameraOpen || !activeCamera || activeCamera.kind === 'local') return;
  const target = activeNetworkCameraImage();
  const url = `/api/cameras/${activeCamera.id}/stream?t=${Date.now()}`;
  ['#cameraNetwork', '#enrollCameraNetwork'].map($).filter(Boolean).forEach(image => {
    if (image === target) image.src = url;
    else image.removeAttribute('src');
  });
}

function localVideoConstraints(camera) {
  const video = {
    width: {ideal: Number(camera.width || 1280)},
    height: {ideal: Number(camera.height || 720)},
    frameRate: {ideal: Number(camera.fps || 30)}
  };
  if (camera.device_id) video.deviceId = {exact: camera.device_id};
  else video.facingMode = 'user';
  return {video, audio: false};
}

async function startCamera() {
  if (cameraOpen) {
    if (activeCamera && activeCamera.kind !== 'local') {
      switchCameraViews(true);
      startNetworkPreview();
    } else {
      await attachCameraViews();
    }
    updateCameraControls();
    return;
  }

  try {
    resetRecognitionSession();
    activeCamera = await preferredCamera();

    if (activeCamera.kind !== 'local') {
      stream = null;
      await fetchCameraFrameBlob(activeCamera.id);
      cameraOpen = true;
      switchCameraViews(true);
      startNetworkPreview();
    } else {
      switchCameraViews(false);
      try {
        if (!window.isSecureContext) {
          throw new Error('远程浏览器调用本机摄像头需要 HTTPS 安全连接；请使用 FaceSign 的 https:// 地址，并先安装 FaceSign 根证书');
        }
        stream = await navigator.mediaDevices.getUserMedia(localVideoConstraints(activeCamera));
      } catch (e) {
        if (activeCamera.device_id && (e.name === 'NotFoundError' || e.name === 'OverconstrainedError')) {
          const fallback = {...activeCamera, device_id: ''};
          stream = await navigator.mediaDevices.getUserMedia(localVideoConstraints(fallback));
          toast('默认本机摄像头未找到，已临时使用系统默认摄像头');
        } else {
          throw e;
        }
      }
      cameraOpen = true;
      await attachCameraViews();
    }

    updateCameraControls();
    const liveState = $('#enrollLiveState');
    if (liveState) {
      liveState.textContent = '摄像头已打开';
      liveState.className = 'capture-state ready';
    }
    const faceHint = $('#enrollFaceHint');
    if (faceHint) faceHint.textContent = '拖动圆圈调整位置，将脸部完整放入引导框；双击可恢复居中';
    if (typeof setEnrollmentStatus === 'function') {
      setEnrollmentStatus(
        supplementStudent ? '准备补充样本' : '摄像头已就绪',
        supplementStudent ? '调整到右侧选择的角度后拍照。' : '请正对摄像头，保持单人入镜。',
        'neutral'
      );
    }
    toast(`${activeCamera.name || '摄像头'}已打开`);
  } catch (e) {
    stream = null;
    cameraOpen = false;
    stopNetworkPreview();
    activeCamera = null;
    switchCameraViews(false);
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
  cameraOpen = false;
  stopNetworkPreview();
  activeCamera = null;
  switchCameraViews(false);
  resetRecognitionSession();
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
  if (cameraOpen) stopCamera();
  else await startCamera();
}

async function attachCameraViews() {
  switchCameraViews(false);
  const views = ['#camera', '#enrollCamera'].map($).filter(Boolean);
  for (const video of views) {
    if (video.srcObject !== stream) video.srcObject = stream;
    if (video.readyState < 1) {
      await new Promise(resolve => video.addEventListener('loadedmetadata', resolve, {once: true}));
    }
    await video.play();
  }
}

function cameraFrameDimensions(selector = '#camera') {
  if (activeCamera?.kind !== 'local') {
    const image = selector === '#enrollCamera' ? $('#enrollCameraNetwork') : $('#cameraNetwork');
    return {
      width: image?.naturalWidth || Number(activeCamera.width || 1280),
      height: image?.naturalHeight || Number(activeCamera.height || 720)
    };
  }
  const video = $(selector);
  return {
    width: video?.videoWidth || Number(activeCamera?.width || 1280),
    height: video?.videoHeight || Number(activeCamera?.height || 720)
  };
}

async function capture(selector = '#camera') {
  if (!cameraOpen) throw new Error('请先打开摄像头');
  if (activeCamera?.kind !== 'local') {
    return await fetchCameraFrameBlob(activeCamera.id);
  }

  if (!stream) throw new Error('本机摄像头画面尚未准备好');
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
