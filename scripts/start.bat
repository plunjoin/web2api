@echo off
setlocal
rem Run from this script's own folder: in the release package web2api.exe sits
rem next to it, so it is found directly. In the repo this script lives in
rem scripts\, so fall back to the parent folder where web2api.exe is built.
cd /d "%~dp0"
if not exist web2api.exe if exist "..\web2api.exe" cd /d "%~dp0.."

if not exist config.yaml (
  copy /Y config.example.yaml config.yaml >nul
)

web2api.exe -config config.yaml
