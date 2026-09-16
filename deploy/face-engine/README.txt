FaceSign 人脸引擎一键部署
========================

依赖：
- Windows：Docker Desktop 已安装并启动。
- Linux：Docker Engine + Docker Compose 已安装，当前用户可执行 docker。
- 官方 CompreFace 默认镜像要求 x86 CPU 支持 AVX。

Windows：
1. 以 PowerShell 打开本目录。
2. 执行：powershell -ExecutionPolicy Bypass -File .\install-compreface.ps1

Linux：
1. chmod +x ./install-compreface.sh
2. ./install-compreface.sh

脚本默认部署官方 CompreFace 1.2.0，并监听 http://127.0.0.1:8000 。
部署后请打开 CompreFace 控制台，创建 Application 和 Face Recognition Service，复制 API Key。
然后登录 FaceSign 管理端 -> 人脸识别设置，选择 CompreFace、填写服务地址和 API Key，保存后点击“检测服务”。

注意：脚本不会删除已有 CompreFace 数据。数据由 CompreFace Docker volume 持久化管理。
