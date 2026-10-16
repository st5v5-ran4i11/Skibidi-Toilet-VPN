using Microsoft.UI.Dispatching;using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using SkibidiVpn.WinUI.Models;
using SkibidiVpn.WinUI.Services;
namespace SkibidiVpn.WinUI.Views;
public sealed partial class CheckPage : Page
{
 private readonly DispatcherQueue _dispatcher = DispatcherQueue.GetForCurrentThread(); private BackendClient? _backend; public CheckPage(){InitializeComponent();}
 protected override void OnNavigatedTo(Microsoft.UI.Xaml.Navigation.NavigationEventArgs e){base.OnNavigatedTo(e);_backend=e.Parameter as BackendClient;if(_backend!=null)_backend.SnapshotReceived+=OnSnapshot;}
 protected override void OnNavigatedFrom(Microsoft.UI.Xaml.Navigation.NavigationEventArgs e){if(_backend!=null)_backend.SnapshotReceived-=OnSnapshot;base.OnNavigatedFrom(e);}
 private void OnSnapshot(BackendSnapshot s)=>_dispatcher.TryEnqueue(()=>{StatusText.Text=$"Pipeline: {s.Pipeline} · обновлено {s.Timestamp.LocalDateTime:T}";var rows=new List<StageRow>();foreach(var stage in s.Stages.Where(x=>!new[]{"COLLECTOR","PROCESSOR","PARSER"}.Contains(x.Name,StringComparer.OrdinalIgnoreCase))){rows.Add(new StageRow(stage.Name,stage.State,stage.Processed,stage.Success,stage.Failed,stage.Total,stage.Active));}var grouped=s.Stages.Where(x=>new[]{"COLLECTOR","PROCESSOR","PARSER"}.Contains(x.Name,StringComparer.OrdinalIgnoreCase)).ToArray();if(grouped.Length>0){var parser=grouped.FirstOrDefault(x=>x.Name.Equals("PARSER",StringComparison.OrdinalIgnoreCase))??grouped.Last();var detail=string.Join(" · ",grouped.Select(x=>$"{x.Name}: {x.Processed} обработано, {x.Success} успешно, {x.Failed} ошибок"));rows.Add(new StageRow("COLLECT / PARSE",grouped.FirstOrDefault(x=>x.Active)?.State??parser.State,parser.Processed,parser.Success,parser.Failed,parser.Total,grouped.Any(x=>x.Active),detail));}StagesList.ItemsSource=rows.OrderBy(x=>StageOrder(x.Name)).ToArray();});
 private static int StageOrder(string name)=>name switch{"FETCHER"=>0,"COLLECT / PARSE"=>1,"TCP CHECK"=>2,"DEEP CHECK"=>3,"RANKING"=>4,_=>5};
 private async Task Run(string action){try{var result=await _backend!.SendCommandAsync(action);StatusText.Text=$"Команда {action}: {result.Status}";}catch(Exception ex){StatusText.Text=$"Ошибка: {ex.Message}";}}
 private async void Start_Click(object s,RoutedEventArgs e)=>await Run("start"); private async void Pause_Click(object s,RoutedEventArgs e)=>await Run("pause"); private async void Continue_Click(object s,RoutedEventArgs e)=>await Run("continue"); private async void Stop_Click(object s,RoutedEventArgs e)=>await Run("stop");
 public sealed record StageRow(string Name,string State,int Processed,int Success,int Failed,int Total,bool Active,string? DetailOverride=null){public bool IsIndeterminate=>Total<=0&&Active;public double Percent=>Total>0?Math.Clamp(100.0*Processed/Total,0,100):0;public string Detail=>DetailOverride??$"Обработано: {Processed} · успешно: {Success} · ошибки: {Failed} · всего: {(Total<0?"неизвестно":Total)}";}
}
