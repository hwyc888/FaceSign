// Follow image texture locally between slower identity requests. Never infer names
// from motion: identity labels only come from the corresponding server face.
let realtimeTrackingTimer = null;
let realtimeTrackingFrames = [];
let realtimeTrackingBoxes = [];
let realtimeTrackingSequence = 0;
let realtimeTrackingSession = '';
const realtimeTrackingCanvas = document.createElement('canvas');

function stopRealtimeTracking() {
  clearTimeout(realtimeTrackingTimer);
  realtimeTrackingTimer = null;
  realtimeTrackingFrames = [];
  realtimeTrackingBoxes = [];
  realtimeTrackingSession = '';
}

function sampleTrackingFrame() {
  const video = activeCamera?.kind === 'browser' ? $('#camera') : $('#cameraNetworkWebRTC');
  const source = video && video.videoWidth && !video.classList.contains('hidden')
    ? video : $('#cameraNetwork');
  if (!source || !(source.videoWidth || source.naturalWidth)) return null;
  const dimensions = cameraFrameDimensions();
  const width = 320;
  const height = Math.max(1, Math.round(width * dimensions.height / dimensions.width));
  realtimeTrackingCanvas.width = width;
  realtimeTrackingCanvas.height = height;
  const context = realtimeTrackingCanvas.getContext('2d', {willReadFrequently: true});
  context.drawImage(source, 0, 0, width, height);
  const pixels = context.getImageData(0, 0, width, height).data;
  const gray = new Uint8Array(width * height);
  for (let index = 0; index < gray.length; index++) {
    gray[index] = (pixels[index * 4] * 77 + pixels[index * 4 + 1] * 150 + pixels[index * 4 + 2] * 29) >> 8;
  }
  return {sequence: ++realtimeTrackingSequence, time: performance.now(), width, height, gray};
}

function trackTextureBox(previous, current, box) {
  if (previous.width !== current.width || previous.height !== current.height) return null;
  const centerX = Math.round((box.x + box.width / 2) * current.width);
  const centerY = Math.round((box.y + box.height / 2) * current.height);
  const radius = Math.max(3, Math.min(9, Math.round(box.width * current.width / 4)));
  const samples = [];
  let sum = 0;
  let squares = 0;
  for (let vertical = -radius; vertical <= radius; vertical += 2) {
    for (let horizontal = -radius; horizontal <= radius; horizontal += 2) {
      const sampleX = centerX + horizontal;
      const sampleY = centerY + vertical;
      if (sampleX < 0 || sampleY < 0 || sampleX >= previous.width || sampleY >= previous.height) return null;
      const value = previous.gray[sampleY * previous.width + sampleX];
      samples.push({horizontal, vertical, value});
      sum += value;
      squares += value * value;
    }
  }
  if (squares / samples.length - (sum / samples.length) ** 2 < 50) return null;
  const matches = [];
  for (let offsetY = -16; offsetY <= 16; offsetY += 2) {
    for (let offsetX = -16; offsetX <= 16; offsetX += 2) {
      if (centerX + offsetX - radius < 0 || centerY + offsetY - radius < 0 ||
          centerX + offsetX + radius >= current.width || centerY + offsetY + radius >= current.height) continue;
      let difference = 0;
      for (const sample of samples) {
        difference += Math.abs(sample.value - current.gray[
          (centerY + offsetY + sample.vertical) * current.width + centerX + offsetX + sample.horizontal
        ]);
      }
      matches.push({offsetX, offsetY, error: difference / samples.length});
    }
  }
  matches.sort((first, second) => first.error - second.error);
  const best = matches[0];
  if (!best || best.error > 22 || Math.abs(best.offsetX) === 16 || Math.abs(best.offsetY) === 16) return null;
  const alternative = matches.find(match => Math.hypot(match.offsetX - best.offsetX, match.offsetY - best.offsetY) >= 6);
  if (alternative && alternative.error < best.error * 1.15 + 1) return null;
  return {...box, x: box.x + best.offsetX / current.width, y: box.y + best.offsetY / current.height};
}

function advanceTrackingBoxes(previous, current, boxes) {
  const updated = boxes.flatMap(item => {
    if (current.time - item.confirmedAt > 3000) return [];
    const box = trackTextureBox(previous, current, item.box);
    return box ? [{...item, box}] : [];
  });
  // Overlapping tracks are ambiguous; hide names until fresh face recognition.
  return updated.filter((item, index) => !updated.some((other, otherIndex) => {
    if (index === otherIndex) return false;
    const overlapWidth = Math.max(0, Math.min(item.box.x + item.box.width, other.box.x + other.box.width) - Math.max(item.box.x, other.box.x));
    const overlapHeight = Math.max(0, Math.min(item.box.y + item.box.height, other.box.y + other.box.height) - Math.max(item.box.y, other.box.y));
    return overlapWidth * overlapHeight > Math.min(item.box.width * item.box.height, other.box.width * other.box.height) * 0.35;
  }));
}

function runRealtimeTracking() {
  if (!cameraOpen || !$('#autoScan')?.checked || recognitionSessionID !== realtimeTrackingSession) {
    stopRealtimeTracking();
    return;
  }
  try {
    const frame = sampleTrackingFrame();
    const previous = realtimeTrackingFrames.at(-1);
    if (frame) {
      if (previous) realtimeTrackingBoxes = advanceTrackingBoxes(previous, frame, realtimeTrackingBoxes);
      realtimeTrackingFrames.push(frame);
      realtimeTrackingFrames = realtimeTrackingFrames.filter(item => frame.time - item.time <= 4000);
      if (realtimeTrackingBoxes.length) {
        drawFaceOverlay(realtimeTrackingBoxes.map(item => ({...item.face, box: item.box})), [], 1, 1);
      } else if (previous) {
        drawFaceOverlay([], []);
      }
    }
  } catch {
    realtimeTrackingBoxes = [];
    drawFaceOverlay([], []);
  }
  realtimeTrackingTimer = setTimeout(runRealtimeTracking, 50);
}

function beginTrackingReference() {
  // Server-side camera captures are not synchronized to browser preview frames.
  if (activeCamera?.kind !== 'browser' || !$('#autoScan')?.checked) return null;
  if (realtimeTrackingSession !== recognitionSessionID) {
    stopRealtimeTracking();
    realtimeTrackingSession = recognitionSessionID;
    runRealtimeTracking();
  }
  // Sample immediately before capture(), rather than using an older timer frame.
  try {
    const frame = sampleTrackingFrame();
    if (frame) {
      const previous = realtimeTrackingFrames.at(-1);
      if (previous) realtimeTrackingBoxes = advanceTrackingBoxes(previous, frame, realtimeTrackingBoxes);
      realtimeTrackingFrames.push(frame);
    }
    return frame;
  } catch {
    stopRealtimeTracking();
    return null;
  }
}

function applyTrackedRecognition(result, reference) {
  if (!reference || realtimeTrackingSession !== recognitionSessionID || !$('#autoScan')?.checked) return false;
  const frames = realtimeTrackingFrames.filter(frame => frame.sequence >= reference.sequence);
  if (!frames.length || frames[0].sequence !== reference.sequence) {
    realtimeTrackingBoxes = [];
    drawFaceOverlay([], []);
    return true;
  }
  let boxes = (result.faces || []).map(face => ({face, confirmedAt: reference.time, box: {
    x: face.box.x / result.frame_width, y: face.box.y / result.frame_height,
    width: face.box.width / result.frame_width, height: face.box.height / result.frame_height
  }}));
  for (let index = 1; index < frames.length && boxes.length; index++) {
    boxes = advanceTrackingBoxes(frames[index - 1], frames[index], boxes);
  }
  realtimeTrackingBoxes = boxes;
  drawFaceOverlay(boxes.map(item => ({...item.face, box: item.box})), [], 1, 1);
  return true;
}
