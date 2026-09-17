# Faster-Whisper-XXL

## 安装运行时和模型

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\tools\faster-whisper-xxl\install.ps1 -Models medium
```

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\tools\faster-whisper-xxl\install.ps1 -Models both
```

## 中文字幕文件

```text
视频文件名.zh-cn.generated.srt
```

## DeepSeek

```powershell
[Environment]::SetEnvironmentVariable('JAVBOSS_DEEPSEEK_API_KEY', '替换为你的DeepSeek API Key', 'User')
[Environment]::SetEnvironmentVariable('JAVBOSS_DEEPSEEK_API_URL', 'https://api.deepseek.com/chat/completions', 'User')
[Environment]::SetEnvironmentVariable('JAVBOSS_DEEPSEEK_MODEL', 'deepseek-flash', 'User')
```

## 自定义工具目录

```powershell
[Environment]::SetEnvironmentVariable('JAVBOSS_WHISPER_DIR', 'D:\path\to\Faster-Whisper-XXL', 'User')
```
