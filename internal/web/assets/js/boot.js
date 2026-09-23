loadCameraConfigs().catch(() => {});
loadAppSettings()
  .then(() => enterCheckinPageAutoStart())
  .catch(() => {});
$('#attendanceDay').value = today();
updateCameraControls();
loadHealth();
loadClasses().then(() => {
  loadStudents();
  if (typeof loadCheckinSeatBoard === 'function') loadCheckinSeatBoard();
});
loadAttendance();
