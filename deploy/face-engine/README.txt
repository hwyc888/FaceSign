FaceSign 人脸引擎自动部署
========================

完整 Windows/Linux 发布包的主安装脚本会自动调用本目录脚本，无需手工创建
CompreFace Application、Face Recognition Service 或复制 API Key。

前置条件：
- Windows：Docker Desktop 已安装并启动。
- Linux：Docker Engine、Docker Compose、curl、unzip 已安装。
- 官方 CompreFace 默认镜像要求 x86 CPU 支持 AVX。

完整安装：
- Windows（管理员 PowerShell）：.\install-service.ps1
- Linux：sudo ./install.sh

脚本默认部署官方 CompreFace 1.2.0，自动创建 FaceSign / FaceSign Recognition，
验证生成的 API Key，并写入 FaceSign 配置。重复执行会复用现有服务和数据。

仅修复或重新部署人脸引擎时，也可单独执行本目录脚本。自动生成的 CompreFace
管理员凭据仅保存在本机受限文件中；请将该文件纳入安全备份。
