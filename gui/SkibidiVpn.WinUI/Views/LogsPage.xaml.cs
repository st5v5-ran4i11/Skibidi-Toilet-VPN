using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using System.Collections.ObjectModel;
using SkibidiVpn.WinUI.Services;

namespace SkibidiVpn.WinUI.Views;

public sealed partial class LogsPage : Page
{
    private readonly DispatcherQueue _dispatcher = DispatcherQueue.GetForCurrentThread();
    private BackendClient? _backend;
    private CancellationTokenSource? _polling;
    private readonly List<string> _fileLines = new();
    private readonly List<string> _ipcLines = new();
    private string? _lastPollError;
    public ObservableCollection<string> VisibleLogs { get; } = new();

    public LogsPage() => InitializeComponent();

    protected override void OnNavigatedTo(Microsoft.UI.Xaml.Navigation.NavigationEventArgs e)
    {
        base.OnNavigatedTo(e);
        _backend = e.Parameter as BackendClient;
        if (_backend != null) _backend.LogReceived += OnLog;
        _polling = new CancellationTokenSource();
        _ = PollLogsAsync(_polling.Token);
    }

    protected override void OnNavigatedFrom(Microsoft.UI.Xaml.Navigation.NavigationEventArgs e)
    {
        if (_backend != null) _backend.LogReceived -= OnLog;
        _polling?.Cancel();
        _polling?.Dispose();
        _polling = null;
        base.OnNavigatedFrom(e);
    }

    private void OnLog(string line) => _dispatcher.TryEnqueue(() => AddIpc(line));

    private async Task PollLogsAsync(CancellationToken token)
    {
        while (!token.IsCancellationRequested)
        {
            try
            {
                if (_backend?.Connected == true)
                {
                    var response = await _backend.GetLogsAsync(1000, token);
                    _dispatcher.TryEnqueue(() =>
                    {
                        _fileLines.Clear();
                        _fileLines.AddRange(response.Lines.TakeLast(1000));
                        _lastPollError = null;
                        ApplyFilter();
                    });
                }
            }
            catch (OperationCanceledException) when (token.IsCancellationRequested) { break; }
            catch (Exception ex)
            {
                if (_lastPollError != ex.Message)
                {
                    _lastPollError = ex.Message;
                    _dispatcher.TryEnqueue(() => AddIpc("Logs IPC: " + ex.Message));
                }
            }
            try { await Task.Delay(TimeSpan.FromSeconds(2), token); }
            catch (OperationCanceledException) { break; }
        }
    }

    private void AddIpc(string line)
    {
        _ipcLines.Add($"{DateTime.Now:HH:mm:ss} {line}");
        if (_ipcLines.Count > 1000) _ipcLines.RemoveRange(0, _ipcLines.Count - 1000);
        ApplyFilter();
    }

    private void ApplyFilter()
    {
        VisibleLogs.Clear();
        var level = LevelFilter?.SelectedIndex ?? 0;
        var query = SearchBox?.Text ?? "";
        foreach (var line in _fileLines.Concat(_ipcLines)
                     .Where(x => (level == 0 || (level == 1 && x.Contains("ERROR", StringComparison.OrdinalIgnoreCase)) || (level == 2 && x.Contains("WARN", StringComparison.OrdinalIgnoreCase)))
                                 && x.Contains(query, StringComparison.OrdinalIgnoreCase))
                     .TakeLast(1500)) VisibleLogs.Add(line);
    }

    private void Filter_Changed(object sender, object args) => ApplyFilter();
    private async void LoadFile_Click(object sender, RoutedEventArgs e)
    {
        try
        {
            var response = await _backend!.GetLogsAsync(1000);
            _fileLines.Clear();
            _fileLines.AddRange(response.Lines.TakeLast(1000));
            ApplyFilter();
        }
        catch (Exception ex) { AddIpc("Не удалось получить логи: " + ex.Message); }
    }
}
