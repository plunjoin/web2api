# web2api 打包与部署

本文区分两件事：**打包**是在开发机生成目标平台的发布包，**部署**是把对应发布包放到目标机器运行。程序使用纯 Go（CGO_ENABLED=0），不需要 Node.js、SQLite 服务或浏览器运行时。

## 支持的发布目标

| 目标 | 包内程序 | 常用场景 |
|---|---|---|
| Windows x86_64 | web2api.exe | Windows 10/11、Windows Server |
| Windows ARM64 | web2api.exe | Windows on ARM |
| Linux x86_64 | web2api | 大多数 VPS、云服务器 |
| Linux ARM64 | web2api | ARM 云主机、树莓派 64 位 |
| Linux ARMv7 | web2api | 32 位 ARM 设备 |
| macOS Intel | web2api | Intel Mac |
| macOS Apple Silicon | web2api | M1/M2/M3/M4 Mac |
| Docker | 镜像 | Linux、Windows、macOS 上的 Docker |

Linux 和 macOS 发布包是 .tar.gz；Windows 用 PowerShell 打包时是 .zip，在 Unix 构建机上也可以使用 .tar.gz。

## 从源码生成所有平台发布包

在仓库根目录执行。脚本会清空并重新生成 dist/，每个包都包含示例配置和运行所需的文档，不会包含本机的 data/、auth/、cookies/ 或敏感配置。Linux 包内的 config.yaml 是指向 /opt/web2api 的部署模板。

### Windows PowerShell（推荐）

~~~powershell
.\build.ps1 -Version v1.0.0
# 输出：dist\web2api-v1.0.0-windows-amd64.zip 等
~~~

### Linux/macOS/WSL

~~~bash
chmod +x build.sh
./build.sh v1.0.0
# 输出：dist/web2api-v1.0.0-linux-amd64.tar.gz 等
~~~

发布前可在 dist/ 中校验 SHA256SUMS。脚本默认构建以下 7 个目标：windows-amd64、windows-arm64、linux-amd64、linux-arm64、linux-armv7、darwin-amd64、darwin-arm64。

也可以只手动构建一个目标（纯 Go 交叉编译）：

~~~bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o web2api ./cmd/web2api
~~~

## Windows 部署

1. 解压 web2api-*-windows-*.zip，复制 config.example.yaml 为 config.yaml。
2. 编辑 config.yaml；首次启动后在 http://localhost:8800/admin 设置管理员和 Redis。
3. 双击 start.bat，或执行：

~~~powershell
.\web2api.exe -config config.yaml
~~~

数据默认写入程序目录下的 data/、auth/、cookies/。需要开机启动时，可把 start.bat 放入任务计划程序；Windows 服务可使用 NSSM 等服务包装器。

## Linux 二进制部署（systemd）

解压 web2api-*-linux-*.tar.gz 后，包内的 install.sh 会按 uname -m 选择 amd64、arm64 或 armv7 二进制。以 root 或可使用 sudo 的账号执行：

~~~bash
tar -xzf web2api-v1.0.0-linux-amd64.tar.gz
cd web2api-v1.0.0-linux-amd64
bash install.sh
~~~

脚本安装到 /opt/web2api，注册 web2api.service 并设置开机启动。配置文件是 /opt/web2api/config.yaml，数据和凭据目录是 /opt/web2api/data、/opt/web2api/auth、/opt/web2api/cookies。

~~~bash
sudo systemctl status web2api
sudo journalctl -u web2api -f
sudo systemctl restart web2api
~~~

如果不使用 systemd，也可以直接运行：

~~~bash
mkdir -p data auth cookies
cp config.example.yaml config.yaml
./web2api -config config.yaml
~~~

## macOS 部署

1. 选择与 Mac CPU 对应的 darwin-amd64 或 darwin-arm64 包并解压。
2. cp config.example.yaml config.yaml，按需修改代理和端口。
3. 执行 chmod +x web2api && ./web2api -config config.yaml。

首次运行如果被 Gatekeeper 拦截，请在“系统设置 → 隐私与安全性”允许该程序，或确认二进制来自可信的发布源。

## Docker 部署

Docker 适用于上述所有能运行 Docker 的系统。仓库根目录已经提供 Dockerfile 和 docker-compose.yml：

~~~bash
docker compose up -d
docker compose logs -f web2api
~~~

基础 Docker 部署只需要 Compose 文件和环境变量，不需要导入 `config.yaml`。容器使用 web2api-data、web2api-auth、web2api-cookies 三个卷保存数据。需要跨 CPU 构建镜像时可使用 Docker Buildx：

~~~bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t your-registry/web2api:v1.0.0 --push .
~~~

## 启动后的地址与目录

| 地址 | 用途 |
|---|---|
| http://<主机>:8800/admin | 管理台，首次访问完成初始化 |
| http://<主机>:8800/v1 | OpenAI 兼容 API Base URL |
| http://<主机>:8800/v1/models | 模型列表和健康检查 |

生产环境请放行 TCP 8800（防火墙和云安全组），并备份 data/、auth/、cookies/。这些目录可能包含账号凭据，不能提交到 Git 或打进发布包。

