FaceSign 原生 CPU 人脸引擎
========================

目标：新机器不安装 Python、不安装 Docker、不要求 GPU/CUDA。

运行结构：
- FaceSign 主程序：Go
- 人脸引擎：facesign-face-engine（Go 原生可执行文件）
- 推理库：ONNX Runtime CPU 版，随发布包携带
- 模型：YuNet + SFace ONNX，随发布包携带
- 人脸特征：SQLite faces.db

Windows
-------
进入发布包 face-engine\windows 目录，运行：
  powershell -ExecutionPolicy Bypass -File .\install.ps1

脚本会自动提升管理员权限，将原生引擎安装为 Windows Service：
  FaceSignFaceEngine

不需要联网下载 Python/pip，也不需要 Docker Desktop。
如果之前使用过 Python 版 FaceSign 本地引擎，会继续使用同一份：
  C:\ProgramData\FaceSign\face-engine\data\faces.db
所以已经采集的人脸不需要重新录入。

卸载且保留人脸数据：
  powershell -ExecutionPolicy Bypass -File .\uninstall.ps1

只有确定要删除生物特征数据时才使用：
  powershell -ExecutionPolicy Bypass -File .\uninstall.ps1 -RemoveData

Linux
-----
进入发布包 face-engine/linux 目录：
  chmod +x ./install.sh
  sudo ./install.sh

安装后服务名：
  facesign-face-engine.service

默认监听：
  http://127.0.0.1:18081

FaceSign 管理端
---------------
人脸识别引擎：本地 CPU 引擎
服务地址：http://127.0.0.1:18081
API Key：不需要填写
识别相似度：建议先使用 0.72
人脸检测阈值：建议 0.80

保存后点击“检测服务”。正常应显示 CPU 引擎可达，并明确 GPU 不需要。

迁移到新机器
------------
1. 迁移 FaceSign 主数据库 facesign.db。
2. 同时迁移人脸数据库 faces.db。
3. 在新机器安装同版本 FaceSign 和原生人脸引擎。
4. 把数据库放回对应数据目录，启动服务即可。

Windows 默认人脸数据库：
  C:\ProgramData\FaceSign\face-engine\data\faces.db

Linux 默认人脸数据库：
  /var/lib/facesign/face-engine/data/faces.db

运行时不依赖 Python、pip、venv、Docker、CUDA 或独立显卡。
