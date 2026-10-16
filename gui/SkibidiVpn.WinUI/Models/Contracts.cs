using System.Text.Json;
using System.Text.Json.Serialization;
namespace SkibidiVpn.WinUI.Models;
public sealed record IpcEndpoint(int Version, string Address, string Token, int Pid);
public sealed record StageProgress(string Name, int Total, int Processed, int Success, int Failed, int Produced, int Alive, int Workers, bool Active, bool Done, string State);
public sealed record SourceStatus(string Name, string Type, string Url, bool Enabled);
public sealed record ServiceStatus(
    string Id, string State, bool Available,
    [property: JsonPropertyName("pool_size")] int PoolSize,
    [property: JsonPropertyName("pool_nodes")] string[]? PoolNodes,
    string? Primary, string? Secondary, int Standby,
    [property: JsonPropertyName("runtime_state")] string RuntimeState,
    int Pid, int Port,
    [property: JsonPropertyName("latency_ms")] long LatencyMs,
    [property: JsonPropertyName("last_health_check")] DateTimeOffset? LastHealthCheck,
    [property: JsonPropertyName("last_error")] string? LastError,
    string? Message);
public sealed record Capabilities(
    [property: JsonPropertyName("pipeline_control")] bool PipelineControl,
    [property: JsonPropertyName("source_listing")] bool SourceListing,
    [property: JsonPropertyName("settings_read")] bool SettingsRead,
    [property: JsonPropertyName("settings_write")] bool SettingsWrite,
    [property: JsonPropertyName("service_manager")] bool ServiceManager,
    [property: JsonPropertyName("runtime_status")] bool RuntimeStatus);
public sealed record BackendSnapshot(int Version, DateTimeOffset Timestamp, string Backend, string Pipeline, StageProgress[] Stages, SourceStatus[] Sources, ServiceStatus[] Services, Dictionary<string, JsonElement> Pools, Dictionary<string, JsonElement> Runtimes, string Router, Capabilities Capabilities);
public sealed record CommandRequest(
    int Version, string Id, string Action,
    [property: JsonPropertyName("service_id")] string? ServiceId = null,
    [property: JsonPropertyName("network_mode")] string? NetworkMode = null);
public sealed record NetworkModeRequest([property: JsonPropertyName("network_mode")] string NetworkMode);
public sealed record CommandResponse(int Version, string Id, bool Accepted, string Status, string? Error, BackendSnapshot? Snapshot);
public sealed record SettingsSnapshot(int Version,
    [property: JsonPropertyName("network_mode")] string NetworkMode,
    [property: JsonPropertyName("run_share_server")] bool RunShareServer,
    [property: JsonPropertyName("fetcher_timeout")] int FetcherTimeout);
public sealed record SourcesResponse(int Version, SourceStatus[] Sources);
public sealed record LogsResponse(int Version, string[] Lines);
public sealed record LogLine(DateTime Timestamp, string Level, string Message);
