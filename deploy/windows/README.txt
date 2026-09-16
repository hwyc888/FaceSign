FaceSign Windows/amd64 部署包
=============================

前置条件：64 位 Windows（CPU 支持 AVX），Docker Desktop 已安装并启动。

以管理员身份打开 PowerShell，在解压目录运行：
  Set-ExecutionPolicy -Scope Process Bypass
  .\install-service.ps1

脚本会安装 Windows 服务、部署 CompreFace、自动创建人脸识别服务和 API Key，
并验证 FaceSign 与人脸引擎都可用后退出。管理端地址：http://SERVER_IP:8080/

如明确只安装管理平台而暂不启用人脸识别：
  .\install-service.ps1 -SkipFaceEngine

程序目录：C:\Program Files\FaceSign
数据目录：C:\ProgramData\FaceSign
