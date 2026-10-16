using System.Diagnostics;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using SkibidiVpn.WinUI.Models;

namespace SkibidiVpn.WinUI.Services;
public sealed class BackendClient : IDisposable
{
    private readonly HttpClient _http = new() { Timeout = TimeSpan.FromSeconds(8) };
    private readonly HttpClient _eventsHttp = new() { Timeout = Timeout.InfiniteTimeSpan };
    private readonly JsonSerializerOptions _json = new() { PropertyNameCaseInsensitive = true };
    private CancellationTokenSource? _lifetime;
    private Process? _process;
    private string? _root;
    private IpcEndpoint? _endpoint;
    public event Action<bool>? ConnectionChanged;
    public event Action<BackendSnapshot>? SnapshotReceived;
    public event Action<string>? LogReceived;
    public bool Connected => _endpoint != null;
    public string? ProjectRoot => _root;
    public void Start()
    {
        if (_lifetime != null) return;
        _lifetime = new CancellationTokenSource();
        _ = Task.Run(() => ConnectionLoopAsync(_lifetime.Token));
    }
    private async Task ConnectionLoopAsync(CancellationToken token)
    {
        while (!token.IsCancellationRequested)
        {
            try
            {
                if (_endpoint == null)
                {
                    _root = FindProjectRoot();
                    string endpointFile = Path.Combine(_root, "data", "ipc.json");
                    if (!File.Exists(endpointFile)) TryStartBackend(_root);
                    for (int i = 0; i < 30 && !token.IsCancellationRequested && !File.Exists(endpointFile); i++) await Task.Delay(500, token);
                    if (!File.Exists(endpointFile)) LogReceived?.Invoke("Go backend не создал data/ipc.json за 15 секунд. Проверьте запуск vpn-backend.exe и logs/application.log.");
                    if (File.Exists(endpointFile))
                    {
                        var raw = await File.ReadAllTextAsync(endpointFile, token);
                        _endpoint = JsonSerializer.Deserialize<IpcEndpoint>(raw, _json);
                        if (_endpoint != null) { _http.BaseAddress = new Uri("http://" + _endpoint.Address + "/"); _http.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", _endpoint.Token); _eventsHttp.BaseAddress = _http.BaseAddress; _eventsHttp.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", _endpoint.Token); }
                    }
                }
                if (_endpoint != null)
                {
                    using var response = await _http.GetAsync("api/v1/snapshot", token);
                    response.EnsureSuccessStatusCode();
                    var snapshot = await response.Content.ReadFromJsonAsync<BackendSnapshot>(_json, token);
                    if (snapshot != null) SnapshotReceived?.Invoke(snapshot);
                    ConnectionChanged?.Invoke(true);
                    await ReadEventsOnceAsync(token);
                }
                else { ConnectionChanged?.Invoke(false); await Task.Delay(1500, token); }
            }
            catch (OperationCanceledException) when (token.IsCancellationRequested) { break; }
            catch (Exception ex) { var failedEndpoint = _endpoint; _endpoint = null; _http.BaseAddress = null; _http.DefaultRequestHeaders.Authorization = null; _eventsHttp.BaseAddress = null; _eventsHttp.DefaultRequestHeaders.Authorization = null; if (!string.IsNullOrWhiteSpace(_root) && failedEndpoint != null) { bool processAlive = false; try { using var process = Process.GetProcessById(failedEndpoint.Pid); processAlive = !process.HasExited; } catch { } if (!processAlive) { try { File.Delete(Path.Combine(_root, "data", "ipc.json")); } catch { } } } ConnectionChanged?.Invoke(false); LogReceived?.Invoke($"IPC: {ex.Message}"); try { await Task.Delay(1500, token); } catch (OperationCanceledException) { break; } }
        }
    }
    private async Task ReadEventsOnceAsync(CancellationToken token)
    {
        if (_endpoint == null) return;
        using var request = new HttpRequestMessage(HttpMethod.Get, "api/v1/events");
        using var response = await _eventsHttp.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, token);
        response.EnsureSuccessStatusCode();
        using var stream = await response.Content.ReadAsStreamAsync(token);
        using var reader = new StreamReader(stream);
        string? line;
        while (!token.IsCancellationRequested && (line = await reader.ReadLineAsync(token)) != null)
        {
            if (!line.StartsWith("data: ", StringComparison.Ordinal)) continue;
            try
            {
                using var doc = JsonDocument.Parse(line[6..]);
                if (doc.RootElement.TryGetProperty("snapshot", out var snap))
                {
                    var value = snap.Deserialize<BackendSnapshot>(_json);
                    if (value != null) SnapshotReceived?.Invoke(value);
                }
                else if (doc.RootElement.TryGetProperty("pipeline", out _))
                {
                    var value = doc.RootElement.Deserialize<BackendSnapshot>(_json);
                    if (value != null) SnapshotReceived?.Invoke(value);
                }
                else LogReceived?.Invoke(line[6..]);
            }
            catch (JsonException) { LogReceived?.Invoke(line[6..]); }
        }
    }
    public async Task<CommandResponse> SendCommandAsync(string action, string? serviceId = null, string? networkMode = null, CancellationToken token = default)
    {
        if (_endpoint == null) throw new InvalidOperationException("Go backend is not connected.");
        var request = new CommandRequest(1, Guid.NewGuid().ToString("N"), action, serviceId, networkMode);
        using var response = await _http.PostAsJsonAsync("api/v1/commands", request, _json, token);
        var result = await response.Content.ReadFromJsonAsync<CommandResponse>(_json, token);
        if (result == null) throw new InvalidDataException("Backend returned an empty command response.");
        if (!result.Accepted) throw new InvalidOperationException(result.Error ?? result.Status);
        return result;
    }
    public async Task<SourcesResponse> GetSourcesAsync(CancellationToken token = default)
    {
        if (_endpoint == null) throw new InvalidOperationException("Go backend is not connected.");
        return await _http.GetFromJsonAsync<SourcesResponse>("api/v1/sources", _json, token) ?? new SourcesResponse(1, Array.Empty<SourceStatus>());
    }
    public async Task<LogsResponse> GetLogsAsync(int limit = 1000, CancellationToken token = default)
    {
        if (_endpoint == null) throw new InvalidOperationException("Go backend is not connected.");
        return await _http.GetFromJsonAsync<LogsResponse>($"api/v1/logs?limit={Math.Clamp(limit, 1, 2000)}", _json, token) ?? new LogsResponse(1, Array.Empty<string>());
    }
    public async Task<SettingsSnapshot> GetSettingsAsync(CancellationToken token = default)
    {
        if (_endpoint == null) throw new InvalidOperationException("Go backend is not connected.");
        return await _http.GetFromJsonAsync<SettingsSnapshot>("api/v1/settings", _json, token) ?? throw new InvalidDataException("Backend returned no settings.");
    }
    public async Task SetNetworkModeAsync(string mode, CancellationToken token = default)
    {
        if (_endpoint == null) throw new InvalidOperationException("Go backend is not connected.");
        using var response = await _http.PutAsJsonAsync("api/v1/settings/network-mode", new NetworkModeRequest(mode), _json, token);
        response.EnsureSuccessStatusCode();
    }
    private static string FindProjectRoot()
    {
        var candidates = new List<string>();
        string? current = AppContext.BaseDirectory;
        for (int i = 0; i < 8 && current != null; i++, current = Directory.GetParent(current)?.FullName) candidates.Add(current);
        current = Environment.CurrentDirectory;
        for (int i = 0; i < 6 && current != null; i++, current = Directory.GetParent(current)?.FullName) candidates.Add(current);
        foreach (var path in candidates.Distinct()) if (File.Exists(Path.Combine(path, "configs", "settings.yaml"))) return path;
        throw new DirectoryNotFoundException("Не удалось найти корень проекта (configs/settings.yaml). Укажите корень репозитория рядом с приложением или запустите GUI из папки проекта.");
    }
    private void TryStartBackend(string root)
    {
        var candidates = new[] { Path.Combine(root, "vpn-backend.exe"), Path.Combine(root, "cmd", "vpn-backend", "vpn-backend.exe"), Path.Combine(root, "bin", "vpn-backend.exe") };
        var executable = candidates.FirstOrDefault(File.Exists);
        if (executable == null) { LogReceived?.Invoke("Не найден vpn-backend.exe. Соберите его командой go build -o vpn-backend.exe ./cmd/vpn-backend."); return; }
        try
        {
            var startInfo = new ProcessStartInfo(executable, $"--root \"{root}\"") { WorkingDirectory = root, UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true, RedirectStandardError = true };
            _process = new Process { StartInfo = startInfo, EnableRaisingEvents = true };
            _process.OutputDataReceived += (_, e) => { if (!string.IsNullOrWhiteSpace(e.Data)) LogReceived?.Invoke("backend: " + e.Data); };
            _process.ErrorDataReceived += (_, e) => { if (!string.IsNullOrWhiteSpace(e.Data)) LogReceived?.Invoke("backend ERROR: " + e.Data); };
            _process.Exited += (_, _) => LogReceived?.Invoke("vpn-backend.exe завершился.");
            if (_process.Start()) { _process.BeginOutputReadLine(); _process.BeginErrorReadLine(); }
        }
        catch (Exception ex) { LogReceived?.Invoke("Не удалось запустить Go backend: " + ex.Message); }
    }
    public void Dispose() { _lifetime?.Cancel(); _lifetime?.Dispose(); _http.Dispose(); _eventsHttp.Dispose(); }
}
