# FaceSign Camera Agent

Camera Agent is used when the central FaceSign server cannot directly reach a classroom IP camera.

It opens no inbound port. It reads the IP camera inside the classroom LAN and pushes the newest JPEG frame outbound to the central server.

## Setup

1. FaceSign -> Settings -> Camera Management -> add "Client Camera Agent".
2. Set an Agent ID and generate/copy a connection key.
3. Copy camera-agent.example.json to camera-agent.json.
4. Put the same server URL, Agent ID and key into the JSON.
5. Put the classroom camera HTTP snapshot or MJPEG URL and camera credentials in the JSON.
6. Run: FaceSignCameraAgent.exe --config camera-agent.json --once
7. Run scripts\install-camera-agent.ps1 as Administrator.

Camera credentials stay in the classroom PC's local configuration file. The installer restricts the installed JSON ACL to SYSTEM and local Administrators.

RTSP still requires the camera's HTTP/HTTPS snapshot endpoint for recognition, so this agent remains a small Go-only executable without Python, Docker or FFmpeg.
