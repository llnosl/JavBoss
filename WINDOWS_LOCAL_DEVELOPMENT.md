# Windows 本地源码安装与启动

## 1. 配置 MSYS2 镜像并安装基础工具

```bash
cp -n /etc/pacman.d/mirrorlist.mingw /etc/pacman.d/mirrorlist.mingw.bak
cp -n /etc/pacman.d/mirrorlist.msys /etc/pacman.d/mirrorlist.msys.bak
sed -i "s#mirror.msys2.org/#mirrors.ustc.edu.cn/msys2/#g" /etc/pacman.d/mirrorlist*
grep -n "mirrors.ustc.edu.cn" /etc/pacman.d/mirrorlist.mingw /etc/pacman.d/mirrorlist.msys
pacman -Syyu
pacman -S --needed git curl mingw-w64-x86_64-gcc mingw-w64-x86_64-7zip
```

## 2. 检查环境

```bash
export PATH="/c/Program Files/Go/bin:/c/Program Files/nodejs:/mingw64/bin:$PATH"
hash -r
git --version
go version
node --version
npm --version
x86_64-w64-mingw32-gcc --version
7z i
```

## 3. 下载仓库

```bash
mkdir -p /d/代码
cd /d/代码
git clone https://github.com/llnosl/JavBoss.git JavBoss
cd /d/代码/JavBoss
```

## 4. 配置 Go 和 npm 镜像

```bash
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn
go env -w CGO_ENABLED=1
go env -w CC=x86_64-w64-mingw32-gcc
npm config set registry https://registry.npmmirror.com
go env GOPROXY GOSUMDB CGO_ENABLED CC
npm config get registry
```

## 5. 安装后端依赖

```bash
cd /d/代码/JavBoss
go mod download
```

## 6. 安装 CLI 依赖

```bash
cd /d/代码/JavBoss/scripts/cli
npm install
npm run build
```

## 7. 安装前端依赖

```bash
cd /d/代码/JavBoss/web
npm install
```

## 8. 安装 ffprobe 和 mpv

```bash
cd /d/代码/JavBoss
bash ./scripts/cli.sh download-dependencies windows-x86_64
```

## 9. 安装 Faster-Whisper-XXL 和全部模型

```bash
cd /d/代码/JavBoss
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./tools/faster-whisper-xxl/install.ps1 -Models both
```

## 10. 只安装 medium 模型

```bash
cd /d/代码/JavBoss
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./tools/faster-whisper-xxl/install.ps1 -Models medium
```

## 11. 永久保存 DeepSeek 配置

```bash
read -rsp "DeepSeek API Key: " JAVBOSS_DEEPSEEK_API_KEY
echo
setx.exe JAVBOSS_DEEPSEEK_API_KEY "$JAVBOSS_DEEPSEEK_API_KEY"
setx.exe JAVBOSS_DEEPSEEK_API_URL "https://api.deepseek.com/chat/completions"
setx.exe JAVBOSS_DEEPSEEK_MODEL "deepseek-flash"
export JAVBOSS_DEEPSEEK_API_KEY
export JAVBOSS_DEEPSEEK_API_URL="https://api.deepseek.com/chat/completions"
export JAVBOSS_DEEPSEEK_MODEL="deepseek-flash"
```

## 12. 启动后端

```bash
cd /d/代码/JavBoss
export PATH="/c/Program Files/Go/bin:/c/Program Files/nodejs:/mingw64/bin:$PATH"
export JAVBOSS_DEEPSEEK_API_KEY="$(cmd.exe /c echo %JAVBOSS_DEEPSEEK_API_KEY% 2>/dev/null | tr -d '\r')"
export JAVBOSS_DEEPSEEK_API_URL="$(cmd.exe /c echo %JAVBOSS_DEEPSEEK_API_URL% 2>/dev/null | tr -d '\r')"
export JAVBOSS_DEEPSEEK_MODEL="$(cmd.exe /c echo %JAVBOSS_DEEPSEEK_MODEL% 2>/dev/null | tr -d '\r')"
bash ./scripts/cli.sh dev backend
```

## 13. 启动前端

```bash
cd /d/代码/JavBoss/web
export PATH="/c/Program Files/nodejs:$PATH"
npm run dev
```

## 14. 验证服务

```bash
curl -i http://127.0.0.1:17654/healthz
curl -I http://127.0.0.1:5173/
cmd.exe /c start http://127.0.0.1:5173/
```

## 15. 后续启动后端

```bash
cd /d/代码/JavBoss
export PATH="/c/Program Files/Go/bin:/c/Program Files/nodejs:/mingw64/bin:$PATH"
export JAVBOSS_DEEPSEEK_API_KEY="$(cmd.exe /c echo %JAVBOSS_DEEPSEEK_API_KEY% 2>/dev/null | tr -d '\r')"
export JAVBOSS_DEEPSEEK_API_URL="$(cmd.exe /c echo %JAVBOSS_DEEPSEEK_API_URL% 2>/dev/null | tr -d '\r')"
export JAVBOSS_DEEPSEEK_MODEL="$(cmd.exe /c echo %JAVBOSS_DEEPSEEK_MODEL% 2>/dev/null | tr -d '\r')"
bash ./scripts/cli.sh dev backend
```

## 16. 后续启动前端

```bash
cd /d/代码/JavBoss/web
export PATH="/c/Program Files/nodejs:$PATH"
npm run dev
```

## 17. 更新源码和依赖

```bash
cd /d/代码/JavBoss
git pull
go mod download
cd /d/代码/JavBoss/scripts/cli
npm install
npm run build
cd /d/代码/JavBoss/web
npm install
```

## 18. 安装浏览器扩展

```bash
cd /d/代码/JavBoss
cygpath -w "$PWD/browser-extension"
cmd.exe /c start chrome://extensions/
```

1. 打开“开发者模式”。
2. 点击“加载已解压的扩展程序”。
3. 选择 `D:\代码\JavBoss\browser-extension`。
4. 将“JavBoss 助手”固定到浏览器工具栏。

## 19. 配置 CloudDrive2 下载

1. 打开 `http://127.0.0.1:5173/`。
2. 进入“下载”→“下载设置”。
3. 设置“本地下载目录”。
4. 设置“CloudDrive2 地址”。
5. 设置“云端离线目录”。
6. 填写 CloudDrive2 API Token。
7. 保存设置。
8. 确认 CloudDrive2 状态显示“可用”。

## 20. 创建浏览器扩展 API 令牌

1. 打开 JavBoss“全局设置”→“安全”。
2. 在“浏览器扩展 API 令牌”中新建令牌。
3. 复制以 `jbe_` 开头的令牌。

## 21. 开启浏览器磁力链接下载

1. 点击浏览器工具栏中的“JavBoss 助手”。
2. Server 地址填写 `http://127.0.0.1:17654`。
3. API 令牌填写上一步复制的 `jbe_` 令牌。
4. 点击“测试连接”。
5. 允许扩展访问 `127.0.0.1`。
6. 开启“启用磁力下载”。
7. 刷新需要点击磁力链接的网页。

## 22. 验证浏览器磁力链接下载

1. 在支持的网站点击包含 BTIH 的磁力链接。
2. 在确认弹窗中点击“确定”。
3. 打开 JavBoss“下载”→“下载任务”。
4. 确认任务进入下载队列。
5. 番号已经存在时，选择“覆盖”或“取消”。
