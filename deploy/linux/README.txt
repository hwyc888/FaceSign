FaceSign Linux/amd64 部署包
===========================

前置条件：x86-64 Linux（CPU 支持 AVX），Docker Engine、Docker Compose、curl、unzip。

安装：
  sudo ./install.sh

脚本会安装 systemd 服务、部署 CompreFace、自动创建人脸识别服务和 API Key，
并验证 FaceSign 与人脸引擎都可用后退出。管理端地址：http://SERVER_IP:8080/

如明确只安装管理平台而暂不启用人脸识别：
  sudo ./install.sh --skip-face-engine

数据目录：/var/lib/facesign
配置文件：/etc/facesign/facesign.env
服务状态：systemctl status facesign
