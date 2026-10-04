@echo off
setlocal

if not exist config.yaml (
  copy /Y config.example.yaml config.yaml >nul
)

web2api.exe -config config.yaml
