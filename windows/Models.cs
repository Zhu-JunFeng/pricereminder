using System.Text.Json.Serialization;

namespace PriceReminder.Windows;

internal sealed record Contract(
    [property: JsonPropertyName("symbol")] string Symbol,
    [property: JsonPropertyName("baseAsset")] string BaseAsset,
    [property: JsonPropertyName("quoteAsset")] string QuoteAsset,
    [property: JsonPropertyName("tickSize")] string TickSize);

internal sealed class AlertRule
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public string Symbol { get; set; } = "BTCUSDT";
    public int WindowMinutes { get; set; } = 5;
    public string ThresholdText { get; set; } = "3";
    public bool Enabled { get; set; } = true;
    public bool RiseTriggered { get; set; }
    public bool FallTriggered { get; set; }
    public AlertRuleKind Kind { get; set; } = AlertRuleKind.Percentage;
    public TargetDirection? TargetDirection { get; set; }
    public string? TargetPriceText { get; set; }
    public bool TargetTriggered { get; set; }

    [JsonIgnore]
    public decimal Threshold => decimal.Parse(ThresholdText, System.Globalization.CultureInfo.InvariantCulture);
    [JsonIgnore]
    public decimal? TargetPrice => decimal.TryParse(TargetPriceText, System.Globalization.NumberStyles.Number,
        System.Globalization.CultureInfo.InvariantCulture, out var value) ? value : null;
}

internal sealed record PricePoint(string Symbol, string PriceText, long EventTime, bool Replay = false)
{
    [JsonIgnore]
    public decimal Price => decimal.Parse(PriceText, System.Globalization.CultureInfo.InvariantCulture);
}

internal enum TriggerDirection { Rise, Fall }
internal enum AlertRuleKind { Percentage, Target }
internal enum TargetDirection { Above, Below }

internal sealed record AlertTrigger(
    Guid RuleId, string Symbol, AlertRuleKind Kind, TriggerDirection Direction, decimal? ChangePercent,
    string ThresholdText, int WindowMinutes, string? TargetPriceText,
    string PriceText, string BaselinePriceText, long EventTime);

internal sealed record TriggerHistory(
    string Id, string Symbol, AlertRuleKind Kind, TriggerDirection Direction, decimal? ChangePercent,
    int WindowMinutes, string ThresholdText, string? TargetPriceText, string PriceText, long EventTime);

internal enum ConnectionSource { Direct, Server }

internal sealed record MonitorSnapshot(
    bool Running,
    bool Connected,
    bool WarmingUp,
    ConnectionSource Source,
    string Message,
    IReadOnlyDictionary<string, PricePoint> Prices,
    IReadOnlySet<string> StaleSymbols,
    DateTimeOffset? LastReceivedAt,
    int ReconnectCount,
    string? LastError,
    int SubscribedCount);

internal sealed class PersistedState
{
    public string PrimarySymbol { get; set; } = "BTCUSDT";
    public List<string> TraySymbols { get; set; } = ["BTCUSDT"];
    public List<AlertRule> Rules { get; set; } = [];
    public List<TriggerHistory> History { get; set; } = [];
    public Dictionary<string, List<PricePoint>> Prices { get; set; } = [];
    public string? DeviceToken { get; set; }
}
