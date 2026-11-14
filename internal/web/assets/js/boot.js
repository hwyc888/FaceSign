function startAuthenticatedApp() {
  loadCameraConfigs().catch(() => {});
  loadRecognitionStats().catch(() => {});
  loadAppSettings()
    .then(() => enterCheckinPageAutoStart())
    .catch(() => {});
  $('#attendanceDay').value = today();
  updateCameraControls();
  loadHealth();
  loadClasses().then(classesLoaded => {
    loadStudents();
    if (classesLoaded && typeof loadCheckinSeatBoard === 'function') loadCheckinSeatBoard();
  });
  loadAttendance();
}

initializeAuth().catch(error => {
  const message = $('#authError');
  if (message) message.textContent = error.message || '无法连接 FaceSign 服务';
});
