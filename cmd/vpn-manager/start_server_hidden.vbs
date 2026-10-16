Set WshShell = CreateObject("WScript.Shell")
' Запускаем powershell в режиме полной невидимости (параметр 0 в конце)
WshShell.Run "powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File share_server.ps1", 0, False
