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

function median(values) {
  const sorted = values.slice().sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)] || 0;
}

function trackTexturePoint(previous, current, point, radius, maxOffset) {
  const centerX = Math.round(point.x * previous.width);
  const centerY = Math.round(point.y * previous.height);
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
  if (squares / samples.length - (sum / samples.length) ** 2 < 35) return null;

  let best = null;
  const matches = [];
  for (let offsetY = -maxOffset; offsetY <= maxOffset; offsetY += 2) {
    for (let offsetX = -maxOffset; offsetX <= maxOffset; offsetX += 2) {
      if (centerX + offsetX - radius < 0 || centerY + offsetY - radius < 0 ||
          centerX + offsetX + radius >= current.width || centerY + offsetY + radius >= current.height) continue;
      let difference = 0;
      for (const sample of samples) {
        difference += Math.abs(sample.value - current.gray[
          (centerY + offsetY + sample.vertical) * current.width + centerX + offsetX + sample.horizontal
        ]);
      }
      const candidate = {offsetX, offsetY, error: difference / samples.length};
      matches.push(candidate);
      if (!best || candidate.error < best.error) best = candidate;
    }
  }
  if (!best || best.error > 24 || Math.abs(best.offsetX) === maxOffset || Math.abs(best.offsetY) === maxOffset) return null;
  let alternative = null;
  for (const candidate of matches) {
    if (Math.hypot(candidate.offsetX - best.offsetX, candidate.offsetY - best.offsetY) >= 6 &&
        (!alternative || candidate.error < alternative.error)) alternative = candidate;
  }
  if (alternative && alternative.error < best.error * 1.12 + 1) return null;
  return best;
}

function trackTextureBox(previous, current, box) {
  if (previous.width !== current.width || previous.height !== current.height) return null;
  const elapsed = current.time - previous.time;
  if (elapsed <= 0 || elapsed > 250) return null;

  const centerX = box.x + box.width / 2;
  const centerY = box.y + box.height / 2;
  const radius = Math.max(2, Math.min(5, Math.round(
    Math.min(box.width * current.width, box.height * current.height) / 16
  )));
  const maxOffset = Math.max(14, Math.min(22, Math.round(12 + elapsed / 40)));
  const points = [
    {role: 'center', x: centerX, y: centerY},
    {role: 'left', x: box.x + box.width * 0.25, y: centerY},
    {role: 'right', x: box.x + box.width * 0.75, y: centerY},
    {role: 'top', x: centerX, y: box.y + box.height * 0.25},
    {role: 'bottom', x: centerX, y: box.y + box.height * 0.75}
  ];
  const matches = new Map();
  for (const point of points) {
    const match = trackTexturePoint(previous, current, point, radius, maxOffset);
    if (match) matches.set(point.role, match);
  }
  if (matches.size < 3) return null;

  const dx = median([...matches.values()].map(match => match.offsetX));
  const dy = median([...matches.values()].map(match => match.offsetY));
  let scaleX = 1;
  let scaleY = 1;
  const left = matches.get('left');
  const right = matches.get('right');
  const top = matches.get('top');
  const bottom = matches.get('bottom');
  if (left && right) {
    const span = Math.max(1, box.width * current.width * 0.5);
    scaleX = Math.max(0.85, Math.min(1.15, 1 + (right.offsetX - left.offsetX) / span));
  }
  if (top && bottom) {
    const span = Math.max(1, box.height * current.height * 0.5);
    scaleY = Math.max(0.85, Math.min(1.15, 1 + (bottom.offsetY - top.offsetY) / span));
  }

  const width = box.width * scaleX;
  const height = box.height * scaleY;
  const nextCenterX = centerX + dx / current.width;
  const nextCenterY = centerY + dy / current.height;
  return {x: nextCenterX - width / 2, y: nextCenterY - height / 2, width, height};
}

function advanceTrackingBoxes(previous, current, boxes) {
  const updated = boxes.flatMap(item => {
    if (current.time - item.confirmedAt > 3000) return [];
    const box = trackTextureBox(previous, current, item.box);
    return box ? [{...item, box}] : [];
  });
  // Only overlapping face boxes can exchange identity labels. Keep body boxes
  // independently visible even when their outlines overlap.
  return updated.filter((item, index) => {
    if (item.kind === 'person') return true;
    return !updated.some((other, otherIndex) => {
      if (index === otherIndex || other.kind === 'person') return false;
      const overlapWidth = Math.max(0, Math.min(item.box.x + item.box.width, other.box.x + other.box.width) - Math.max(item.box.x, other.box.x));
      const overlapHeight = Math.max(0, Math.min(item.box.y + item.box.height, other.box.y + other.box.height) - Math.max(item.box.y, other.box.y));
      return overlapWidth * overlapHeight > Math.min(item.box.width * item.box.height, other.box.width * other.box.height) * 0.35;
    });
  });
}

function realtimeTrackingEnabled() {
  return Boolean($('#autoScan')?.checked || manualRecognitionBurst);
}

function drawRealtimeTrackingBoxes() {
  const faces = realtimeTrackingBoxes
    .filter(item => item.kind !== 'person')
    .map(item => ({...item.face, box: item.box}));
  const persons = realtimeTrackingBoxes
    .filter(item => item.kind === 'person')
    .map(item => ({...item.person, box: item.box}));
  drawFaceOverlay(faces, persons, 1, 1);
}

function runRealtimeTracking() {
  if (!cameraOpen || recognitionSessionID !== realtimeTrackingSession ||
      !$('#page-checkin')?.classList.contains('active')) {
    stopRealtimeTracking();
    drawFaceOverlay([], []);
    return;
  }
  if (!realtimeTrackingEnabled()) {
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
      if (realtimeTrackingBoxes.length) drawRealtimeTrackingBoxes();
      else if (previous) drawFaceOverlay([], []);
    }
  } catch {
    realtimeTrackingBoxes = [];
    drawFaceOverlay([], []);
  }
  realtimeTrackingTimer = setTimeout(runRealtimeTracking, 33);
}

function beginTrackingReference() {
  // Server-side camera captures are not synchronized to browser preview frames.
  if (activeCamera?.kind !== 'browser' || !cameraOpen || !realtimeTrackingEnabled()) return null;
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
  if (!reference || realtimeTrackingSession !== recognitionSessionID || !realtimeTrackingEnabled()) return false;
  const frames = realtimeTrackingFrames.filter(frame => frame.sequence >= reference.sequence);
  if (!frames.length || frames[0].sequence !== reference.sequence) {
    realtimeTrackingBoxes = [];
    drawFaceOverlay([], []);
    return true;
  }
  if (!result.frame_width || !result.frame_height) return false;
  const boxes = [];
  for (const face of result.faces || []) {
    boxes.push({
      kind: 'face',
      face,
      confirmedAt: reference.time,
      box: {
        x: face.box.x / result.frame_width, y: face.box.y / result.frame_height,
        width: face.box.width / result.frame_width, height: face.box.height / result.frame_height
      }
    });
  }
  for (const person of result.persons || []) {
    boxes.push({
      kind: 'person',
      person,
      confirmedAt: reference.time,
      box: {
        x: person.box.x / result.frame_width, y: person.box.y / result.frame_height,
        width: person.box.width / result.frame_width, height: person.box.height / result.frame_height
      }
    });
  }
  let tracked = boxes;
  for (let index = 1; index < frames.length && tracked.length; index++) {
    tracked = advanceTrackingBoxes(frames[index - 1], frames[index], tracked);
  }
  if (!tracked.length) return false;
  realtimeTrackingBoxes = tracked;
  drawRealtimeTrackingBoxes();
  return true;
}
