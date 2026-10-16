Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

# ============================================================
# АВТОМАТИЧЕСКИЙ РАСЧЕТ КОРНЯ ПРОЕКТА (УМНЫЕ ПУТИ)
# ============================================================
# Скрипт лежит в: ./cmd/vpn-manager/share_server.ps1
# 1-й шаг вверх:  ./cmd/vpn-manager
# 2-й шаг вверх:  ./cmd/ (корень проекта)
$DirManager = Split-Path $PSScriptRoot -Parent
$ProjectRoot = Split-Path $DirManager -Parent

$Port = 8000
$ShareDir = $ProjectRoot  # Сервер Python будет раздавать корень проекта, чтобы папка output была доступна по сети!

# Настройки проверки Python
$python = Get-Command python -ErrorAction SilentlyContinue
if (-not $python) {
    [System.Windows.Forms.MessageBox]::Show("Python не найден в PATH.", "Skibidi_toilet_VPN", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
    exit
}

# ============================================================
# АВТОМАТИЧЕСКИЙ ПОДБОР РАБОЧЕГО АДРЕСА
# ============================================================

# Собираем список всех доступных IPv4 адресов на ПК
$IpCandidates = @()
$PrimaryIp = (Get-NetIPAddress -AddressFamily IPv4 -InterfaceAlias "Ethernet" -ErrorAction SilentlyContinue).IPAddress
if ($PrimaryIp) { $IpCandidates += $PrimaryIp }

$OtherIps = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { 
    $_.IPAddress -notlike "127.*" -and $_.InterfaceAlias -notlike "*Loopback*" -and $_.IPAddress -notin $IpCandidates
}).IPAddress

if ($OtherIps) { $IpCandidates += $OtherIps }
$IpCandidates += "127.0.0.1"

$ServerStarted = $false
$IP = ""
$Server = $null

# Перебор адресов с надежной проверкой через сокеты
foreach ($CurrentIP in $IpCandidates) {
    Write-Host "Попытка запуска на адресе: http://$CurrentIP`:$Port/" -ForegroundColor Cyan
    
    $ProcessInfo = New-Object System.Diagnostics.ProcessStartInfo
    $ProcessInfo.FileName = $python.Source
    $ProcessInfo.Arguments = "-m http.server $Port --bind $CurrentIP"
    $ProcessInfo.WorkingDirectory = $ShareDir
    $ProcessInfo.UseShellExecute = $true
    $ProcessInfo.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden

    $Server = New-Object System.Diagnostics.Process
    $Server.StartInfo = $ProcessInfo

    try {
        $Server.Start() | Out-Null
        Start-Sleep -Milliseconds 600 # Пауза для старта сокета Python
        
        # Проверяем, открылся ли порт
        $TcpClient = New-Object System.Net.Sockets.TcpClient
        $Connect = $TcpClient.BeginConnect($CurrentIP, $Port, $null, $null)
        $Wait = $Connect.AsyncWaitHandle.WaitOne(1000, $false)
        
        if ($Wait -and $TcpClient.Connected) {
            $TcpClient.EndConnect($Connect)
            $TcpClient.Close()
            $IP = $CurrentIP
            $ServerStarted = $true
            break
        }
        else {
            $TcpClient.Close()
        }
    }
    catch {
        if ($Server -and -not $Server.HasExited) { $Server.Kill() }
    }
    
    if ($Server -and -not $Server.HasExited) { $Server.Kill() }
}

# Если сервер вообще нигде не отвечает
if (-not $ServerStarted) {
    [System.Windows.Forms.MessageBox]::Show(
        "Не удалось запустить работающий сервер ни на одном из адресов.`n`nВозможно, порт $Port уже занят другим приложением.",
        "Критическая ошибка сокета",
        [System.Windows.Forms.MessageBoxButtons]::OK,
        [System.Windows.Forms.MessageBoxIcon]::Error
    )
    exit
}

Write-Host "Сервер успешно запущен и проверен на: http://$IP`:$Port/" -ForegroundColor Green

# ============================================================
# ИКОНКА В ТРЕЕ
# ============================================================

$NotifyIcon = New-Object System.Windows.Forms.NotifyIcon
$IconPath = Join-Path $ProjectRoot "icons/serv.ico"

if (Test-Path $IconPath) { $NotifyIcon.Icon = New-Object System.Drawing.Icon($IconPath) }
else { $NotifyIcon.Icon = [System.Drawing.SystemIcons]::Information }
$NotifyIcon.Text = "🚽 Skibidi toilet VPN"
$NotifyIcon.Visible = $true

# ============================================================
# ФУНКЦИЯ ДЛЯ ЧИСТОГО ЗАКРЫТИЯ
# ============================================================

$Script:ShutdownAll = {
    if ($script:Server -and -not $script:Server.HasExited) {
        $script:Server.Kill()
        $script:Server.WaitForExit()
    }
    $script:NotifyIcon.Visible = $false
    $script:NotifyIcon.Dispose()
    [System.Windows.Forms.Application]::Exit()
    Stop-Process -Id $PID
}

# ============================================================
# КОНТЕКСТНОЕ МЕНЮ ТРЕЯ
# ============================================================

$Menu = New-Object System.Windows.Forms.ContextMenuStrip

$OpenServer = New-Object System.Windows.Forms.ToolStripMenuItem
$PathServerIco = Join-Path $ProjectRoot "icons/server.ico"
if (Test-Path $PathServerIco) { $OpenServer.Image = [System.Drawing.Image]::FromFile($PathServerIco) }
$OpenServer.Text = "Открыть сервер"
$OpenServer.Add_Click({ Start-Process "http://$($script:IP):$($script:Port)/" })
$Menu.Items.Add($OpenServer) | Out-Null

$OpenSubscription = New-Object System.Windows.Forms.ToolStripMenuItem
$PathAllIco = Join-Path $ProjectRoot "icons/all.ico"
if (Test-Path $PathAllIco) { $OpenSubscription.Image = [System.Drawing.Image]::FromFile($PathAllIco) }
$OpenSubscription.Text = "Открыть all.txt"
$OpenSubscription.Add_Click({ Start-Process "http://$($script:IP):$($script:Port)/output/subscriptions/all.txt" })
$Menu.Items.Add($OpenSubscription) | Out-Null

$OpenFolder = New-Object System.Windows.Forms.ToolStripMenuItem
$PathFolderIco = Join-Path $ProjectRoot "icons/folder.ico"
if (Test-Path $PathFolderIco) { $OpenFolder.Image = [System.Drawing.Image]::FromFile($PathFolderIco) }
$OpenFolder.Text = "Открыть папку"
$OpenFolder.Add_Click({ Start-Process explorer.exe $script:ShareDir })
$Menu.Items.Add($OpenFolder) | Out-Null

$Separator = New-Object System.Windows.Forms.ToolStripSeparator
$Menu.Items.Add($Separator) | Out-Null

$StopServer = New-Object System.Windows.Forms.ToolStripMenuItem
$PathStopIco = Join-Path $ProjectRoot "icons/stop.ico"
if (Test-Path $PathStopIco) { $StopServer.Image = [System.Drawing.Image]::FromFile($PathStopIco) }
$StopServer.Text = "Остановить сервер"
$StopServer.Add_Click($Script:ShutdownAll)
$Menu.Items.Add($StopServer) | Out-Null

$NotifyIcon.ContextMenuStrip = $Menu

# ============================================================
# ДЕЙСТВИЯ С ИКОНКОЙ
# ============================================================

$NotifyIcon.Add_DoubleClick({ Start-Process "http://$($script:IP):$($script:Port)/" })

$NotifyIcon.BalloonTipTitle = "Skibidi_toilet_VPN"
$NotifyIcon.BalloonTipText = "Сервер успешно поднят!`nАдрес: http://$IP`:$Port/"
$NotifyIcon.BalloonTipIcon = [System.Windows.Forms.ToolTipIcon]::Info
$NotifyIcon.ShowBalloonTip(3000)

[System.Windows.Forms.Application]::Run()
