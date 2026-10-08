param(
    [string]$Version = "dev",
    [string]$Output = "dist"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Dist = Join-Path $Root $Output
$Targets = @(
    @{ Name = "windows-amd64"; GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe"; Format = "zip" },
    @{ Name = "windows-arm64"; GOOS = "windows"; GOARCH = "arm64"; Ext = ".exe"; Format = "zip" },
    @{ Name = "linux-amd64";   GOOS = "linux";   GOARCH = "amd64"; Ext = "";     Format = "tar.gz" },
    @{ Name = "linux-arm64";   GOOS = "linux";   GOARCH = "arm64"; Ext = "";     Format = "tar.gz" },
    @{ Name = "linux-armv7";   GOOS = "linux";   GOARCH = "arm";   GOARM = "7"; Ext = ""; Format = "tar.gz" },
    @{ Name = "darwin-amd64";  GOOS = "darwin";  GOARCH = "amd64"; Ext = "";     Format = "tar.gz" },
    @{ Name = "darwin-arm64";  GOOS = "darwin";  GOARCH = "arm64"; Ext = "";     Format = "tar.gz" }
)

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "未找到 Go，请先安装 Go 1.27 或更高版本。"
}

New-Item -ItemType Directory -Force -Path $Dist | Out-Null
Get-ChildItem -LiteralPath $Dist -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
$env:CGO_ENABLED = "0"
$env:GOCACHE = Join-Path $Root ".gocache-build"

foreach ($target in $Targets) {
    $name = "web2api-$Version-$($target.Name)"
    $stage = Join-Path $Dist $name
    New-Item -ItemType Directory -Force -Path $stage | Out-Null

    $env:GOOS = $target.GOOS
    $env:GOARCH = $target.GOARCH
    if ($target.GOARM) { $env:GOARM = $target.GOARM } else { Remove-Item Env:GOARM -ErrorAction SilentlyContinue }

    $binaryName = if ($target.GOOS -eq "windows") { "web2api.exe" } else { "web2api" }
    & go build -trimpath -ldflags "-s -w -X web2api/internal/version.Version=$Version" -o (Join-Path $stage $binaryName) .
    if ($LASTEXITCODE -ne 0) { throw "构建失败: $($target.Name)" }

    Copy-Item (Join-Path $Root "config.example.yaml") (Join-Path $stage "config.example.yaml")
    Copy-Item (Join-Path $Root "README.md") (Join-Path $stage "README.md")
    if ($target.GOOS -eq "windows") {
        Copy-Item (Join-Path $Root "start.bat") (Join-Path $stage "start.bat")
    } elseif ($target.GOOS -eq "linux") {
        Copy-Item (Join-Path $Root "deploy/README.md") (Join-Path $stage "DEPLOY.md")
        Copy-Item (Join-Path $Root "deploy/install.sh") (Join-Path $stage "install.sh")
        Copy-Item (Join-Path $Root "deploy/web2api.service") (Join-Path $stage "web2api.service")
        Copy-Item (Join-Path $Root "deploy/config.yaml") (Join-Path $stage "config.yaml")
    }

    $archive = Join-Path $Dist "$name.$($target.Format)"
    if ($target.Format -eq "zip") {
        Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $archive -Force
    } else {
        tar -czf $archive -C $Dist $name
        if ($LASTEXITCODE -ne 0) { throw "打包失败: $($target.Name)" }
    }
    Remove-Item -LiteralPath $stage -Recurse -Force
    Write-Host "已生成 $archive"
}

Get-ChildItem -LiteralPath $Dist -File | ForEach-Object {
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
    "$hash  $($_.Name)" | Out-File -Encoding ascii -Append (Join-Path $Dist "SHA256SUMS")
}
Write-Host "完成。发布包和 SHA256SUMS 位于 $Dist"
