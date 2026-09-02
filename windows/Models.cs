using System.Text.Json.Serialization;

namespace PriceReminder.Windows;

internal sealed record Contract(
    [property: JsonPropertyName("symbol")] string Symbol,
    [property: JsonPropertyName("baseAsset")] string BaseAsset,
    [property: JsonPropertyName("quoteAsset")] string QuoteAsset,
    [property: JsonPropertyName("tickSize")] string TickSize);

internal static class ContractOrdering
{
    public static IReadOnlyList<Contract> Ordered(
        IEnumerable<Contract> contracts, IEnumerable<string> recentSymbols, string query = "")
    {
        var normalized = query.Trim();
        var ranks = recentSymbols.Take(3).Select((symbol, index) => (symbol, index))
            .GroupBy(item => item.symbol, StringComparer.OrdinalIgnoreCase)
            .ToDictionary(group => group.Key, group => group.First().index, StringComparer.OrdinalIgnoreCase);
        return contracts.Select((contract, index) => (contract, index))
            .Where(item => normalized.Length == 0
                || item.contract.Symbol.Contains(normalized, StringComparison.OrdinalIgnoreCase)
                || item.contract.BaseAsset.Contains(normalized, StringComparison.OrdinalIgnoreCase)
                || item.contract.QuoteAsset.Contains(normalized, StringComparison.OrdinalIgnoreCase))
            .OrderBy(item => ranks.TryGetValue(item.contract.Symbol, out var rank) ? rank : int.MaxValue)
            .ThenBy(item => item.index)
            .Select(item => item.contract).ToList();
    }
}

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
internal enum AlertRuleKind { Percentage = 0, Target = 1, MarketPercentage = 2 }
internal enum TargetDirection { Above, Below }

internal sealed record AlertTrigger(
    Guid RuleId, string Symbol, AlertRuleKind Kind, TriggerDirection Direction, decimal? ChangePercent,
    string ThresholdText, int WindowMinutes, string? TargetPriceText,
    string PriceText, string BaselinePriceText, long EventTime);

internal sealed class MarketAlertRule
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public int WindowMinutes { get; set; } = 5;
    public string ThresholdText { get; set; } = "3";
    public bool Enabled { get; set; } = true;

    [JsonIgnore]
    public decimal Threshold => decimal.Parse(ThresholdText, System.Globalization.CultureInfo.InvariantCulture);
}

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
    int SubscribedCount,
    string MarketMessage,
    int MarketContractCount,
    DateTimeOffset? LastMarketReceivedAt);

internal sealed class PersistedState
{
    public string PrimarySymbol { get; set; } = "BTCUSDT";
    public List<string> TraySymbols { get; set; } = ["BTCUSDT"];
    public List<string> RecentSymbols { get; set; } = [];
    public List<AlertRule> Rules { get; set; } = [];
    public List<MarketAlertRule> MarketRules { get; set; } = [];
    public List<TriggerHistory> History { get; set; } = [];
    public Dictionary<string, List<PricePoint>> Prices { get; set; } = [];
    public string? DeviceToken { get; set; }
}
