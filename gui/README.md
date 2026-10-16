# SkibidiVpn.WinUI

Native WinUI 3 desktop front end. Go remains the sole owner of pipeline, pools, runtime lifecycle, routing and configuration logic.

## Requirements

- Windows 10 2004 (build 19041) or Windows 11 x64/ARM64.
- .NET 8 SDK.
- Visual Studio 2022 with **.NET desktop development**, **WinUI application development**, Windows 10/11 SDK and MSBuild components, or equivalent Build Tools.
- Windows App SDK 1.6.250205002 restored by NuGet, plus Windows App Runtime 1.6 installed on the target machine (this project is unpackaged and framework-dependent).
- The Go version declared in the repository's `go.mod` (currently 1.26.6) to build the backend.

## Build from repository root (PowerShell)

```powershell
go build -o .\vpn-backend.exe .\cmd\vpn-backend
dotnet restore .\gui\SkibidiVpn.WinUI\SkibidiVpn.WinUI.csproj
dotnet build .\gui\SkibidiVpn.WinUI\SkibidiVpn.WinUI.csproj -c Release -p:Platform=x64
.\gui\SkibidiVpn.WinUI\bin\x64\Release\net8.0-windows10.0.19041.0\win-x64\SkibidiVpn.WinUI.exe
```

The GUI locates the project root by searching parent directories for `configs/settings.yaml`. It starts `vpn-backend.exe` if found and if no IPC endpoint is present. The backend writes an authenticated loopback endpoint to `data/ipc.json`. The file contains a per-process bearer token and should not be shared. The HTTP server binds only to `127.0.0.1` and validates the token on every API route.

Closing the GUI closes its IPC client but intentionally leaves the Go backend running so that the next GUI launch can reconnect. To stop the backend, terminate the specific `vpn-backend.exe` process after stopping the pipeline/services. Pipeline STOP cancels the pipeline and managed runtimes; it does not terminate the IPC host.

## Current integration boundary

Pipeline START/PAUSE/CONTINUE/STOP, Progress Manager snapshots, service pool/runtime/router snapshots, service reconnect, source configuration listing, reading settings and changing `network_mode` are wired to Go. Stopping an individual service is disabled because the current router API does not support transactional removal of only one service route. The UI does not show fake pools, latency or readiness. `RUNNING` is distinct from `ACTIVE`, which requires a recorded successful health-check.
