let networkPreviewRetryTimer = null;
let networkPreviewGeneration = 0;
let networkPreviewPeer = null;
let networkPreviewWatchdogTimer = null;

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
  ['#cameraNetwork', '#enrollCameraNetwork', '#cameraNetworkWebRTC', '#enrollCameraNetworkWebRTC']
    .map($).filter(Boolean).forEach(view => view.classList.add('hidden'));
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

function activeNetworkCameraVideo() {
  if ($('#page-students')?.classList.contains('active')) return $('#enrollCameraNetworkWebRTC');
  return $('#cameraNetworkWebRTC');
}

function closeNetworkPreviewPeer() {
  const peer = networkPreviewPeer;
  networkPreviewPeer = null;
  if (!peer) return;
  peer.ontrack = null;
  peer.onconnectionstatechange = null;
  try {
    peer.close();
  } catch {}
}

function clearNetworkPreviewTimers() {
  if (networkPreviewRetryTimer) {
    clearTimeout(networkPreviewRetryTimer);
    networkPreviewRetryTimer = null;
  }
  if (networkPreviewWatchdogTimer) {
    clearTimeout(networkPreviewWatchdogTimer);
    networkPreviewWatchdogTimer = null;
  }
}

function stopNetworkPreview() {
  networkPreviewGeneration++;
  clearNetworkPreviewTimers();
  closeNetworkPreviewPeer();

  ['#cameraNetwork', '#enrollCameraNetwork'].map($).filter(Boolean).forEach(image => {
    image.onerror = null;
    image.removeAttribute('src');
    image.classList.add('hidden');
  });
  ['#cameraNetworkWebRTC', '#enrollCameraNetworkWebRTC'].map($).filter(Boolean).forEach(video => {
    video.pause();
    video.srcObject = null;
    video.classList.add('hidden');
  });
}

function waitForICEGatheringComplete(peer, timeoutMS = 4000) {
  if (peer.iceGatheringState === 'complete') return Promise.resolve();
  return new Promise((resolve, reject) => {
    let finished = false;
    const finish = error => {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
      peer.removeEventListener('icegatheringstatechange', onChange);
      if (error) reject(error);
      else resolve();
    };
    const onChange = () => {
      if (peer.iceGatheringState === 'complete') finish();
    };
    const timer = setTimeout(() => finish(new Error('WebRTC ICE 候选收集超时')), timeoutMS);
    peer.addEventListener('icegatheringstatechange', onChange);
  });
}

function startMJPEGPreviewFallback(generation, image, video, reason = '') {
  if (generation !== networkPreviewGeneration || !cameraOpen || !activeCamera || activeCamera.kind === 'local') {
    return;
  }

  clearNetworkPreviewTimers();
  closeNetworkPreviewPeer();
  if (video) {
    video.pause();
    video.srcObject = null;
    video.classList.add('hidden');
  }
  image.classList.remove('hidden');

  const reconnect = () => {
    if (generation !== networkPreviewGeneration || !cameraOpen || !activeCamera || activeCamera.kind === 'local') {
      return;
    }
    image.src = `/api/cameras/${activeCamera.id}/stream?t=${Date.now()}`;
  };

  image.onerror = () => {
    if (generation !== networkPreviewGeneration || !cameraOpen) return;
    if (networkPreviewRetryTimer) clearTimeout(networkPreviewRetryTimer);
    networkPreviewRetryTimer = setTimeout(() => {
      networkPreviewRetryTimer = null;
      reconnect();
    }, 800);
  };
  reconnect();

  if (reason) {
    console.warn('WebRTC H.264 preview unavailable; using MJPEG fallback:', reason);
  }
}

async function startWebRTCH264Preview(generation, image, video) {
  if (!window.RTCPeerConnection) throw new Error('当前浏览器不支持 WebRTC');
  const peer = new RTCPeerConnection();
  networkPreviewPeer = peer;
  peer.addTransceiver('video', {direction: 'recvonly'});

  peer.ontrack = event => {
    if (generation !== networkPreviewGeneration || peer !== networkPreviewPeer) return;
    const remote = event.streams?.[0] || new MediaStream([event.track]);
    video.srcObject = remote;
    image.classList.add('hidden');
    video.classList.remove('hidden');
    video.play().catch(error => console.warn('WebRTC preview play failed', error));
  };

  peer.onconnectionstatechange = () => {
    if (generation !== networkPreviewGeneration || peer !== networkPreviewPeer) return;
    const state = peer.connectionState;
    if (state === 'failed' || state === 'closed') {
      startMJPEGPreviewFallback(generation, image, video, `WebRTC 状态：${state}`);
      return;
    }
    if (state === 'disconnected') {
      if (networkPreviewRetryTimer) clearTimeout(networkPreviewRetryTimer);
      networkPreviewRetryTimer = setTimeout(() => {
        networkPreviewRetryTimer = null;
        if (generation === networkPreviewGeneration && peer === networkPreviewPeer && peer.connectionState === 'disconnected') {
          startMJPEGPreviewFallback(generation, image, video, 'WebRTC 连接中断');
        }
      }, 1500);
    }
  };

  const offer = await peer.createOffer();
  await peer.setLocalDescription(offer);
  await waitForICEGatheringComplete(peer);
  if (generation !== networkPreviewGeneration || peer !== networkPreviewPeer) return;

  const local = peer.localDescription;
  if (!local) throw new Error('WebRTC offer 未生成');
  const response = await fetch(`/api/cameras/${activeCamera.id}/webrtc`, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    cache: 'no-store',
    body: JSON.stringify({type: local.type, sdp: local.sdp})
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.error || `WebRTC 返回 HTTP ${response.status}`);
  }
  const answer = await response.json();
  await peer.setRemoteDescription(answer);

  networkPreviewWatchdogTimer = setTimeout(() => {
    networkPreviewWatchdogTimer = null;
    if (generation !== networkPreviewGeneration || peer !== networkPreviewPeer) return;
    if (!video.videoWidth || !video.videoHeight || video.readyState < 2) {
      startMJPEGPreviewFallback(generation, image, video, 'WebRTC 已连接但未收到可播放 H.264 画面');
    }
  }, 6000);
}

function startNetworkPreview() {
  if (!cameraOpen || !activeCamera || activeCamera.kind === 'local') return;
  const image = activeNetworkCameraImage();
  const video = activeNetworkCameraVideo();
  if (!image || !video) return;

  const generation = ++networkPreviewGeneration;
  clearNetworkPreviewTimers();
  closeNetworkPreviewPeer();

  ['#cameraNetwork', '#enrollCameraNetwork'].map($).filter(Boolean).forEach(view => {
    view.onerror = null;
    view.removeAttribute('src');
    view.classList.add('hidden');
  });
  ['#cameraNetworkWebRTC', '#enrollCameraNetworkWebRTC'].map($).filter(Boolean).forEach(view => {
    view.pause();
    view.srcObject = null;
    view.classList.add('hidden');
  });

  if (String(activeCamera.protocol || '').toLowerCase() !== 'rtsp') {
    startMJPEGPreviewFallback(generation, image, video);
    return;
  }

  startWebRTCH264Preview(generation, image, video).catch(error => {
    if (generation !== networkPreviewGeneration) return;
    startMJPEGPreviewFallback(generation, image, video, error.message);
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
    if (activeCamera?.kind !== 'local') {
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
  if (activeCamera && activeCamera.kind !== 'local') {
    const webrtcVideo = selector === '#enrollCamera' ? $('#enrollCameraNetworkWebRTC') : $('#cameraNetworkWebRTC');
    if (webrtcVideo && !webrtcVideo.classList.contains('hidden') && webrtcVideo.videoWidth && webrtcVideo.videoHeight) {
      return {width: webrtcVideo.videoWidth, height: webrtcVideo.videoHeight};
    }
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
