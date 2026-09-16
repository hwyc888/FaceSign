FaceSign 本地 CPU 人脸引擎
========================

默认推荐方案：Local CPU Engine
- 不需要独立显卡
- 不需要 CUDA
- 不需要 Docker / Docker Desktop
- 使用 OpenCV YuNet + SFace ONNX 模型，仅使用 CPU
- 默认只监听 127.0.0.1:18081

Windows
-------
管理员 PowerShell 运行：
  powershell -ExecutionPolicy Bypass -File .\install-localcpu.ps1

脚本会自动：
1. 在 C:\ProgramData\FaceSign\face-engine 安装独立 Python 3.11 运行环境；
2. 安装 CPU 版 OpenCV / FastAPI 依赖；
3. 下载 YuNet 和 SFace ONNX 模型；
4. 注册 FaceSignFaceEngine 开机启动任务；
5. 启动并检测 http://127.0.0.1:18081/health。

不要求系统预装 Python，也不要求安装 Docker。
卸载：
  powershell -ExecutionPolicy Bypass -File .\uninstall-localcpu.ps1
默认保留已经采集的人脸特征数据。加 -RemoveData 才删除数据。

Linux
-----
执行：
  chmod +x ./install-localcpu.sh
  ./install-localcpu.sh

脚本会创建独立 venv 和 systemd 服务 facesign-face-engine，并监听 127.0.0.1:18081。
不需要 GPU 或 Docker。

FaceSign 管理端设置
------------------
人脸识别引擎：本地 CPU 引擎（推荐，无需 GPU / Docker）
服务地址：http://127.0.0.1:18081
API Key：留空
相似度阈值：建议先使用 0.72
人脸检测阈值：建议 0.80
保存后点击“检测服务”。

可选方案
--------
如果已有独立 CompreFace 服务，FaceSign 仍然保留 CompreFace Provider；该模式需要填写 CompreFace 服务地址与 API Key，但 Windows 默认部署不再依赖 CompreFace 或 Docker。
