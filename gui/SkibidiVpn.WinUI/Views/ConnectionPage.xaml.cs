using Microsoft.UI.Dispatching;using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using SkibidiVpn.WinUI.Models;
using SkibidiVpn.WinUI.Services;
namespace SkibidiVpn.WinUI.Views;
public sealed partial class ConnectionPage : Page
{
    private readonly DispatcherQueue _dispatcher = DispatcherQueue.GetForCurrentThread();
    private BackendClient? _backend;
    public ConnectionPage() { InitializeComponent(); Loaded += (_, _) => RefreshSnapshot(); }
    protected override void OnNavigatedTo(Microsoft.UI.Xaml.Navigation.NavigationEventArgs e) { base.OnNavigatedTo(e); _backend = e.Parameter as BackendClient; if (_backend != null) _backend.SnapshotReceived += OnSnapshot; RefreshSnapshot(); }
    protected override void OnNavigatedFrom(Microsoft.UI.Xaml.Navigation.NavigationEventArgs e) { if (_backend != null) _backend.SnapshotReceived -= OnSnapshot; base.OnNavigatedFrom(e); }
    private void OnSnapshot(BackendSnapshot s) => _dispatcher.TryEnqueue(() => Render(s));
    private void Render(BackendSnapshot s) { StatusText.Text = $"Backend: {s.Backend} · IPC v{s.Version}"; PipelineText.Text = $"Pipeline: {s.Pipeline}"; var active = s.Stages.FirstOrDefault(x => x.Active); StageText.Text = active == null ? "Активный этап: —" : $"Активный этап: {active.Name} · обработано {active.Processed} · успешно {active.Success} · ошибок {active.Failed}"; ServicesList.ItemsSource = s.Services.Select(x => new ServiceRow(x.Id, $"{x.Id} — {x.State}", $"Пул: {x.PoolSize} · резерв: {x.Standby} · primary: {x.Primary ?? "—"} · runtime: {x.RuntimeState} · узлы: {string.Join(", ", x.PoolNodes ?? Array.Empty<string>())}" + (x.LastHealthCheck.HasValue ? $" · проверка: {x.LastHealthCheck.Value.LocalDateTime:T} · {x.LatencyMs} мс" : " · успешный health-check ещё не подтверждён") + (string.IsNullOrWhiteSpace(x.LastError) ? "" : $" · ошибка: {x.LastError}"))).ToArray(); ServiceInfo.Text = s.Capabilities.ServiceManager ? "Пулы и runtime-состояния поступают от Go Service Manager. ACTIVE выставляется только после успешного health-check; RUNNING процесса сам по себе не считается готовностью." : "Service Manager недоступен."; RuntimeText.Text = $"Router: {s.Router}. Статус процесса — {string.Join(", ", s.Services.Select(x => $"{x.Id}: {x.RuntimeState} (PID {x.Pid})"))}"; }
    private void RefreshSnapshot() { if (_backend?.Connected != true) { StatusText.Text = "Go backend не подключён. Проверьте журнал в разделе «Логи»."; return; } }
    private async void Refresh_Click(object sender, RoutedEventArgs e) { try { await _backend!.SendCommandAsync("get-status"); } catch (Exception ex) { StatusText.Text = ex.Message; } }
    private async void Reconnect_Click(object sender, RoutedEventArgs e) { try { if (ServicesList.SelectedItem is not ServiceRow selected) { StatusText.Text = "Выберите сервис."; return; } var result = await _backend!.SendCommandAsync("reconnect-service", selected.Id); StatusText.Text = $"{selected.Id}: {result.Status}"; } catch (Exception ex) { StatusText.Text = "Переподключение: " + ex.Message; } }
    public sealed record ServiceRow(string Id, string Summary, string Detail);
}
