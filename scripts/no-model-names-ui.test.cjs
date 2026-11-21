const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const assets = path.join(__dirname, '..', 'internal', 'web', 'assets');
const read = relative => fs.readFileSync(path.join(assets, relative), 'utf8');

test('visible FaceSign pages do not expose implementation model names', () => {
  const files = [
    'index.html',
    'js/navigation.js',
    'js/face_compare.js',
    'js/recognition.js',
    'style.css'
  ];
  const forbidden = [
    /ArcFace/i,
    /SCRFD/i,
    /MiniFASNet/i,
    /V1SE/i,
    /YOLOX/i,
    /anti-spoof-mn3/i,
    /ResNet50/i
  ];
  for (const file of files) {
    const source = read(file);
    for (const pattern of forbidden) {
      assert.doesNotMatch(source, pattern, file + ' must not expose ' + pattern);
    }
  }
});

test('header shows service state while retaining useful runtime information', () => {
  const html = read('index.html');
  const navigation = read('js/navigation.js');
  const css = read('style.css');
  assert.match(html, /id="healthServiceStatus"/);
  assert.match(html, /id="healthServiceDetails"/);
  assert.match(navigation, /人脸识别服务正常/);
  assert.match(navigation, /h\.device/);
  assert.match(navigation, /h\.match_threshold/);
  assert.match(navigation, /h\.students/);
  assert.match(css, /\.service-health-pill/);
});

test('face comparison and live recognition do not render model metadata', () => {
  const compare = read('js/face_compare.js');
  const recognition = read('js/recognition.js');
  assert.doesNotMatch(compare, /result\.engine|result\.model_version/);
  assert.match(compare, /人脸库学生/);
  assert.doesNotMatch(recognition, /r\.liveness_model|活体模型/);
  assert.match(recognition, /活体检测 · 人体跟踪/);
});

test('system status describes recognition capabilities without naming engines', () => {
  const html = read('index.html');
  assert.match(html, /识别流程：人脸检测与身份比对/);
  assert.match(html, /活体检测：连续 3 帧验证/);
  assert.match(html, /人脸特征相似度需达到该值/);
  assert.match(html, /人脸检测得分需达到该门槛/);
});
