using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Windowing;
using Microsoft.UI.Dispatching;
using Windows.Graphics;
using WinRT.Interop;
using SkibidiVpn.WinUI.Services;
using SkibidiVpn.WinUI.Views;

namespace SkibidiVpn.WinUI;
public sealed partial class MainWindow : Window
{
    private readonly BackendClient _backend = new();
    private readonly DispatcherQueue _dispatcher = DispatcherQueue.GetForCurrentThread();
    public MainWindow()
    {
        InitializeComponent();
        Closed += (_, _) => _backend.Dispose();
        Title = "Skibidi Toilet VPN";
        var hwnd = WindowNative.GetWindowHandle(this);
        var windowId = Win32Interop.GetWindowIdFromWindow(hwnd);
        AppWindow.GetFromWindowId(windowId).Resize(new SizeInt32(960, 680));
        Navigation.SelectedItem = Navigation.MenuItems[0];
        ContentFrame.Navigate(typeof(ConnectionPage), _backend);
        _backend.ConnectionChanged += connected => _dispatcher.TryEnqueue(() =>
        {
            IpcIndicator.Fill = new SolidColorBrush(connected ? Colors.LimeGreen : Colors.Gray);
            IpcStatusText.Text = connected ? "IPC: подключено" : "IPC: нет соединения";
            BackendStatusText.Text = connected ? "Backend: готов" : "Backend: отключён";
        });
        _backend.Start();
    }
    private void Navigation_SelectionChanged(NavigationView sender, NavigationViewSelectionChangedEventArgs args)
    {
        if (args.SelectedItem is not NavigationViewItem item || item.Tag is not string tag) return;
        Type? page = tag switch { "connection" => typeof(ConnectionPage), "check" => typeof(CheckPage), "sources" => typeof(SourcesPage), "settings" => typeof(SettingsPage), "logs" => typeof(LogsPage), _ => null };
        if (page != null) ContentFrame.Navigate(page, _backend);
    }
}
