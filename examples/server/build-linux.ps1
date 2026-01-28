# Cross-compile for Linux AMD64
$env:GOOS="linux"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"
go build -o kcpq-server-linux main.go
Write-Host "Build complete! Checking file type..."
file kcpq-server-linux 2>$null
Get-Item kcpq-server-linux | Select-Object Name, @{Name="SizeMB";Expression={[math]::Round($_.Length/1MB,2)}}
