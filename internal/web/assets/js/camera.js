let networkPreviewRetryTimer = null;
let networkPreviewGeneration = 0;
let networkPreviewPeer = null;
let networkPreviewWatchdogTimer = null;

let cameraRealtimeStatusEnabled = true;
let cameraRealtimeStatusFields = {
  mode: true,
  video: true,
  drop: true,
  network: true,
  recognition: true,
  reason: true
};
let cameraRealtimeStatusTimer = null;
let cameraRealtimeStatusBusy = false;
let cameraRealtimeVideo = null;
let cameraRealtimeMode = '关闭';
let cameraRealtimeReason = '';
let cameraRealtimeStartedAt = 0;
let cameraRealtimeLastVideoQuality = null;
let cameraRealtimeLastRTPStats = null;
let cameraRealtimeRecognitionSamples = [];

function setCameraRealtimeField(name, text, state = '') {
  document.querySelectorAll(`[data-camera-stat="${name}"]`).forEach(node => {
    node.textContent = text;
    node.classList.remove('good', 'warn', 'bad');
    if (state) node.classList.add(state);
    node.classList.toggle('hidden', cameraRealtimeStatusFields[name] === false);
  });
}

function friendlyCameraRealtimeReason(reason) {
  const original = String(reason || '').trim();
  if (!original) return '';
  let text = original.replace(/^WebRTC H\.264 预览不可用[:：]\s*/i, '').trim();
  const lower = text.toLowerCase();

  const ffmpegMissing =
    lower.includes('未找到 ffmpeg') ||
    lower.includes('facesign_ffmpeg 指定的 ffmpeg 不存在') ||
    lower.includes('release package is missing ffmpeg') ||
    lower.includes('ffmpeg.exe 不存在');
  if (ffmpegMissing) {
    return '未找到 ffmpeg.exe；请使用 facesign-windows-amd64-full / lite 完整解压运行，不要只复制 FaceSign.exe';
  }
  if (lower.includes('sha-256') || lower.includes('sha256')) {
    return '摄像头 RTSP Digest 使用 SHA256；请在海康认证设置中把 RTSP Digest 算法改为 MD5 后重试';
  }
  if (lower.includes('401 unauthorized') || text.includes('RTSP 认证失败') || text.includes('认证失败（401）')) {
    return 'RTSP 认证失败（401）；请检查海康 RTSP 用户名、密码和 RTSP Digest 认证设置';
  }
  if (text.includes('H.265') || lower.includes('hevc') || lower.includes('h265')) {
    if (text.includes('转码') || text.includes('编码器')) {
      return '已检测到 H.265/HEVC，但本机 H.264 转码能力不可用；请使用带 FFmpeg 的 Full/Lite 版本';
    }
    return '检测到 H.265/HEVC；FaceSign 将自动转码为 H.264 后通过 WebRTC 实时显示';
  }
  if (lower.includes('connection refused') || text.includes('端口拒绝连接')) {
    return 'RTSP 端口拒绝连接；请确认摄像头 RTSP 服务已启用，并核对 554/实际 RTSP 端口';
  }
  if (lower.includes('timed out') || lower.includes('timeout') || text.includes('连接超时')) {
    return 'RTSP 连接超时；请检查摄像头 RTSP 端口、网络/VLAN 和摄像头 RTSP 服务';
  }
  if (lower.includes('404') || text.includes('路径不存在')) {
    return 'RTSP 路径不存在；FaceSign 已尝试海康 101/102/ISAPI 兼容路径，请检查设备 RTSP 服务';
  }
  if (text.includes('ICE')) {
    return 'WebRTC ICE 协商失败或超时，请检查本机网络/防火墙';
  }
  if (text.includes('未收到可播放 H.264 画面')) {
    return 'WebRTC 已连接，但没有收到可播放的 H.264 视频帧';
  }
  if (text.includes('未检测到 H.264')) {
    return 'RTSP 已连接但未检测到 H.264，请检查摄像头主/子码流编码';
  }
  if (text.includes('检测 RTSP 编码失败')) {
    return 'RTSP 编码检测失败；请检查 RTSP 地址、账号、密码和端口';
  }
  if (text.includes('连接中断')) {
    return 'WebRTC 连接中断，已自动回退到 MJPEG';
  }
  if (lower.includes('状态：failed') || lower.includes('状态: failed')) {
    return 'WebRTC 连接失败，已自动回退到 MJPEG';
  }
  if (lower.includes('状态：closed') || lower.includes('状态: closed')) {
    return 'WebRTC 连接已关闭，已自动回退到 MJPEG';
  }

  text = text.replace(/^FFmpeg[:：]\s*/i, '').trim();
  return text.length > 160 ? `${text.slice(0, 160)}…` : text;
}

function renderCameraRealtimeStatus(values = {}) {
  const mode = values.mode || cameraRealtimeMode || '关闭';
  setCameraRealtimeField('mode', `通道：${mode}`, mode === '关闭' ? '' : mode.includes('回退') ? 'warn' : 'good');
  setCameraRealtimeField('video', values.video || '显示：-- FPS', values.videoState || '');
  setCameraRealtimeField('drop', values.drop || '丢帧：--', values.dropState || '');
  setCameraRealtimeField('network', values.network || '网络：--', values.networkState || '');
  setCameraRealtimeField('recognition', values.recognition || '识别：0.0 FPS', values.recognitionState || '');

  const reason = friendlyCameraRealtimeReason(cameraRealtimeReason);
  document.querySelectorAll('[data-camera-stat="reason"]').forEach(node => {
    node.textContent = reason ? `原因：${reason}` : '原因：--';
    node.classList.toggle('hidden', !reason || cameraRealtimeStatusFields.reason === false);
    node.classList.toggle('bad', Boolean(reason));
  });
  document.querySelectorAll('[data-camera-realtime-status]').forEach(node => {
    node.title = reason ? `摄像头实时运行状态：${reason}` : '摄像头实时运行状态';
    node.classList.toggle('hidden', !cameraRealtimeStatusEnabled);
  });
}

function setCameraRealtimeStatusFields(fields = {}) {
  cameraRealtimeStatusFields = {
    ...cameraRealtimeStatusFields,
    ...fields
  };
  renderCameraRealtimeStatus({mode: cameraRealtimeMode});
}

function setCameraRealtimeStatusEnabled(enabled) {
  cameraRealtimeStatusEnabled = Boolean(enabled);
  document.querySelectorAll('[data-camera-realtime-status]').forEach(node => {
    node.classList.toggle('hidden', !cameraRealtimeStatusEnabled);
  });
  if (!cameraRealtimeStatusEnabled) {
    if (cameraRealtimeStatusTimer) {
      clearInterval(cameraRealtimeStatusTimer);
      cameraRealtimeStatusTimer = null;
    }
    return;
  }
  renderCameraRealtimeStatus({mode: cameraRealtimeMode});
  if (cameraOpen) {
    ensureCameraRealtimeStatusTimer();
    updateCameraRealtimeStatus();
  }
}

function cameraRealtimeRecognitionMetrics(now) {
  const cutoff = now - 2500;
  cameraRealtimeRecognitionSamples = cameraRealtimeRecognitionSamples.filter(sample => sample.at >= cutoff);
  if (!cameraRealtimeRecognitionSamples.length) return {fps: 0, latency: 0};
  const windowSeconds = Math.max(.5, Math.min(2.5, (now - Math.max(cameraRealtimeStartedAt, cutoff)) / 1000));
  const fps = cameraRealtimeRecognitionSamples.length / windowSeconds;
  const latency = cameraRealtimeRecognitionSamples.reduce((sum, sample) => sum + sample.duration, 0) /
    cameraRealtimeRecognitionSamples.length;
  return {fps, latency};
}

function recordRecognitionRealtimeSample(durationMS) {
  if (!cameraOpen) return;
  const now = performance.now();
  cameraRealtimeRecognitionSamples.push({at: now, duration: Math.max(0, Number(durationMS || 0))});
}

function ensureCameraRealtimeStatusTimer() {
  if (!cameraRealtimeStatusEnabled || cameraRealtimeStatusTimer) return;
  cameraRealtimeStatusTimer = setInterval(updateCameraRealtimeStatus, 1000);
}

function startCameraRealtimeVideoMonitor(video, mode, reason = '') {
  cameraRealtimeVideo = video || null;
  cameraRealtimeMode = mode || '连接中';
  cameraRealtimeReason = reason || '';
  cameraRealtimeStartedAt = cameraRealtimeStartedAt || performance.now();
  cameraRealtimeLastVideoQuality = null;
  cameraRealtimeLastRTPStats = null;
  ensureCameraRealtimeStatusTimer();
  renderCameraRealtimeStatus({mode: cameraRealtimeMode});
  updateCameraRealtimeStatus();
}

function stopCameraRealtimeStatus() {
  if (cameraRealtimeStatusTimer) {
    clearInterval(cameraRealtimeStatusTimer);
    cameraRealtimeStatusTimer = null;
  }
  cameraRealtimeStatusBusy = false;
  cameraRealtimeVideo = null;
  cameraRealtimeMode = '关闭';
  cameraRealtimeReason = '';
  cameraRealtimeStartedAt = 0;
  cameraRealtimeLastVideoQuality = null;
  cameraRealtimeLastRTPStats = null;
  cameraRealtimeRecognitionSamples = [];
  renderCameraRealtimeStatus({mode: '关闭'});
}

async function updateCameraRealtimeStatus() {
  if (!cameraRealtimeStatusEnabled || cameraRealtimeStatusBusy || !cameraOpen) return;
  cameraRealtimeStatusBusy = true;
  try {
    const now = performance.now();
    let fps = null;
    let dropPct = null;
    let packetLossPct = null;
    let jitterMS = null;
    let resolution = '';
    const video = cameraRealtimeVideo;

    if (video) {
      if (video.videoWidth && video.videoHeight) resolution = `${video.videoWidth}×${video.videoHeight}`;
      if (typeof video.getVideoPlaybackQuality === 'function') {
        const quality = video.getVideoPlaybackQuality();
        if (cameraRealtimeLastVideoQuality) {
          const seconds = Math.max(.25, (now - cameraRealtimeLastVideoQuality.at) / 1000);
          const total = Math.max(0, quality.totalVideoFrames - cameraRealtimeLastVideoQuality.total);
          const dropped = Math.max(0, quality.droppedVideoFrames - cameraRealtimeLastVideoQuality.dropped);
          if (total > 0) {
            fps = Math.max(0, total - dropped) / seconds;
            dropPct = dropped * 100 / total;
          }
        }
        cameraRealtimeLastVideoQuality = {
          at: now,
          total: quality.totalVideoFrames,
          dropped: quality.droppedVideoFrames
        };
      }
    }

    const peer = networkPreviewPeer;
    if (peer && peer.connectionState === 'connected') {
      const reports = await peer.getStats();
      let inbound = null;
      reports.forEach(report => {
        if (report.type === 'inbound-rtp' && (report.kind === 'video' || report.mediaType === 'video')) inbound = report;
      });
      if (inbound) {
        if (fps === null && Number.isFinite(inbound.framesPerSecond)) fps = Number(inbound.framesPerSecond);
        if (Number.isFinite(inbound.jitter)) jitterMS = Number(inbound.jitter) * 1000;
        const received = Number(inbound.packetsReceived || 0);
        const lost = Number(inbound.packetsLost || 0);
        if (cameraRealtimeLastRTPStats) {
          const receivedDelta = Math.max(0, received - cameraRealtimeLastRTPStats.received);
          const lostDelta = Math.max(0, lost - cameraRealtimeLastRTPStats.lost);
          const packets = receivedDelta + lostDelta;
          if (packets > 0) packetLossPct = lostDelta * 100 / packets;
        }
        cameraRealtimeLastRTPStats = {received, lost};
      }
    }

    const recognition = cameraRealtimeRecognitionMetrics(now);
    let videoState = '';
    if (fps !== null) videoState = fps >= 15 ? 'good' : fps >= 8 ? 'warn' : 'bad';
    let dropState = '';
    if (dropPct !== null) dropState = dropPct <= 1 ? 'good' : dropPct <= 5 ? 'warn' : 'bad';
    let networkState = '';
    if (packetLossPct !== null) networkState = packetLossPct <= 1 ? 'good' : packetLossPct <= 5 ? 'warn' : 'bad';
    const recognizingNow = $('#autoScan')?.checked;
    const recognitionState = recognizingNow ? (recognition.fps >= 2 ? 'good' : recognition.fps > 0 ? 'warn' : 'bad') : '';

    let networkText = '网络：--';
    if (cameraRealtimeMode === 'USB/本机') {
      networkText = '网络：本机';
    } else if (cameraRealtimeMode.includes('MJPEG')) {
      networkText = '网络：HTTP流';
    } else if (packetLossPct !== null || jitterMS !== null) {
      const parts = [];
      if (packetLossPct !== null) parts.push(`丢包 ${packetLossPct.toFixed(1)}%`);
      if (jitterMS !== null) parts.push(`抖动 ${jitterMS.toFixed(0)}ms`);
      networkText = `网络：${parts.join(' · ')}`;
    }

    const videoText = fps === null
      ? `显示：-- FPS${resolution ? ` · ${resolution}` : ''}`
      : `显示：${fps.toFixed(1)} FPS${resolution ? ` · ${resolution}` : ''}`;
    const dropText = dropPct === null ? '丢帧：--' : `丢帧：${dropPct.toFixed(1)}%`;
    const recognitionText = recognition.latency > 0
      ? `识别：${recognition.fps.toFixed(1)} FPS · ${recognition.latency.toFixed(0)}ms`
      : `识别：${recognition.fps.toFixed(1)} FPS`;

    renderCameraRealtimeStatus({
      mode: cameraRealtimeMode,
      video: videoText,
      videoState,
      drop: dropText,
      dropState,
      network: networkText,
      networkState,
      recognition: recognitionText,
      recognitionState
    });
  } catch (error) {
    console.debug('camera realtime stats unavailable', error);
  } finally {
    cameraRealtimeStatusBusy = false;
  }
}

async function toggleCameraFullscreen(event) {
  const media = event.currentTarget;
  const container = media?.closest('[data-camera-fullscreen]');
  if (!container) return;
  event.preventDefault();
  event.stopPropagation();

  try {
    if (document.fullscreenElement) {
      await document.exitFullscreen();
      return;
    }
    await container.requestFullscreen();
  } catch (error) {
    toast('无法切换全屏：' + (error?.message || '浏览器拒绝全屏操作'));
  }
}

function setupCameraFullscreenHandlers() {
  [
    '#camera',
    '#cameraNetworkWebRTC',
    '#cameraNetwork',
    '#enrollCamera',
    '#enrollCameraNetworkWebRTC',
    '#enrollCameraNetwork'
  ].map($).filter(Boolean).forEach(media => {
    media.addEventListener('dblclick', toggleCameraFullscreen);
  });
}

setupCameraFullscreenHandlers();

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

function retryWebRTCH264Compatibility(generation, image, video, reason = '') {
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
  startCameraRealtimeVideoMonitor(null, 'WebRTC H.264兼容重试', reason);

  startWebRTCH264Preview(generation, image, video, true).catch(error => {
    if (generation !== networkPreviewGeneration) return;
    startMJPEGPreviewFallback(
      generation,
      image,
      video,
      error?.message || reason || 'H.264兼容转码失败'
    );
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
  startCameraRealtimeVideoMonitor(null, reason ? 'MJPEG回退' : 'MJPEG', reason);

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

async function startWebRTCH264Preview(generation, image, video, forceTranscode = false) {
  if (!window.RTCPeerConnection) throw new Error('当前浏览器不支持 WebRTC');
  const peer = new RTCPeerConnection();
  let negotiatedMode = forceTranscode ? 'WebRTC H.264兼容转码' : 'WebRTC H.264直通';
  networkPreviewPeer = peer;
  peer.addTransceiver('video', {direction: 'recvonly'});

  peer.ontrack = event => {
    if (generation !== networkPreviewGeneration || peer !== networkPreviewPeer) return;
    const remote = event.streams?.[0] || new MediaStream([event.track]);
    video.srcObject = remote;
    image.classList.add('hidden');
    video.classList.remove('hidden');
    startCameraRealtimeVideoMonitor(video, negotiatedMode);
    video.play().catch(error => console.warn('WebRTC preview play failed', error));
  };

  peer.onconnectionstatechange = () => {
    if (generation !== networkPreviewGeneration || peer !== networkPreviewPeer) return;
    const state = peer.connectionState;
    if (state === 'failed' || state === 'closed') {
      const reason = `WebRTC 状态：${state}`;
      if (!forceTranscode) retryWebRTCH264Compatibility(generation, image, video, reason);
      else startMJPEGPreviewFallback(generation, image, video, reason);
      return;
    }
    if (state === 'disconnected') {
      if (networkPreviewRetryTimer) clearTimeout(networkPreviewRetryTimer);
      networkPreviewRetryTimer = setTimeout(() => {
        networkPreviewRetryTimer = null;
        if (generation === networkPreviewGeneration && peer === networkPreviewPeer && peer.connectionState === 'disconnected') {
          if (!forceTranscode) retryWebRTCH264Compatibility(generation, image, video, 'WebRTC 连接中断');
          else startMJPEGPreviewFallback(generation, image, video, 'WebRTC 连接中断');
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
    body: JSON.stringify({
      type: local.type,
      sdp: local.sdp,
      force_transcode: forceTranscode
    })
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.error || `WebRTC 返回 HTTP ${response.status}`);
  }
  const answer = await response.json();
  negotiatedMode = String(answer.mode || negotiatedMode);
  await peer.setRemoteDescription(answer);

  networkPreviewWatchdogTimer = setTimeout(() => {
    networkPreviewWatchdogTimer = null;
    if (generation !== networkPreviewGeneration || peer !== networkPreviewPeer) return;
    if (!video.videoWidth || !video.videoHeight || video.readyState < 2) {
      const reason = 'WebRTC 已连接但未收到可播放 H.264 画面';
      if (!forceTranscode) retryWebRTCH264Compatibility(generation, image, video, reason);
      else startMJPEGPreviewFallback(generation, image, video, reason);
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
  startCameraRealtimeVideoMonitor(null, '连接中');

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
    retryWebRTCH264Compatibility(generation, image, video, error.message);
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
      const isRTSPNetwork = activeCamera.kind === 'network' &&
        String(activeCamera.protocol || '').toLowerCase() === 'rtsp';
      if (!isRTSPNetwork) {
        await fetchCameraFrameBlob(activeCamera.id);
      }
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
  stopCameraRealtimeStatus();
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
  const activeVideo = $('#page-students')?.classList.contains('active') ? $('#enrollCamera') : $('#camera');
  startCameraRealtimeVideoMonitor(activeVideo, 'USB/本机');
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
