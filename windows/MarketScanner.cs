namespace PriceReminder.Windows;

internal sealed class MarketScanner
{
    private sealed record Sample(long EventTime, decimal Price, string PriceText);
    private sealed class SymbolState(long eventTime)
    {
        public long LastEventTime { get; set; } = eventTime;
        public LinkedList<Sample> Rise { get; } = [];
        public LinkedList<Sample> Fall { get; } = [];
    }

    private readonly Dictionary<Guid, Dictionary<string, SymbolState>> states = [];
    private readonly object gate = new();

    public void RetainRules(IReadOnlySet<Guid> ids)
    {
        lock (gate)
            foreach (var id in states.Keys.Where(id => !ids.Contains(id)).ToArray()) states.Remove(id);
    }

    public void ResetAll()
    {
        lock (gate) states.Clear();
    }

    public IReadOnlyList<AlertTrigger> Evaluate(MarketAlertRule rule, PricePoint current)
    {
        lock (gate) return EvaluateLocked(rule, current);
    }

    private IReadOnlyList<AlertTrigger> EvaluateLocked(MarketAlertRule rule, PricePoint current)
    {
        if (!rule.Enabled || current.Price <= 0) return [];
        if (!states.TryGetValue(rule.Id, out var bySymbol)) states[rule.Id] = bySymbol = new(StringComparer.OrdinalIgnoreCase);
        var sample = new Sample(current.EventTime, current.Price, current.PriceText);
        if (!bySymbol.TryGetValue(current.Symbol, out var state))
        {
            state = new SymbolState(current.EventTime);
            state.Rise.AddLast(sample);
            state.Fall.AddLast(sample);
            bySymbol[current.Symbol] = state;
        }
        if (current.EventTime < state.LastEventTime) return [];
        var cutoff = current.EventTime - rule.WindowMinutes * 60_000L;
        AppendMinimum(state.Rise, sample, cutoff);
        AppendMaximum(state.Fall, sample, cutoff);
        var riseBase = state.Rise.First!.Value;
        var fallBase = state.Fall.First!.Value;
        var riseChange = (current.Price - riseBase.Price) / riseBase.Price * 100m;
        var fallChange = (current.Price - fallBase.Price) / fallBase.Price * 100m;
        var result = new List<AlertTrigger>();
        if (riseChange >= rule.Threshold)
        {
            result.Add(Trigger(rule, current, riseBase, TriggerDirection.Rise, riseChange));
            Reset(state.Rise, sample);
        }
        if (fallChange <= -rule.Threshold)
        {
            result.Add(Trigger(rule, current, fallBase, TriggerDirection.Fall, fallChange));
            Reset(state.Fall, sample);
        }
        state.LastEventTime = current.EventTime;
        return result;
    }

    private static void AppendMinimum(LinkedList<Sample> values, Sample sample, long cutoff)
    {
        Purge(values, cutoff);
        while (values.Last is { } last && last.Value.Price >= sample.Price) values.RemoveLast();
        values.AddLast(sample);
    }

    private static void AppendMaximum(LinkedList<Sample> values, Sample sample, long cutoff)
    {
        Purge(values, cutoff);
        while (values.Last is { } last && last.Value.Price <= sample.Price) values.RemoveLast();
        values.AddLast(sample);
    }

    private static void Purge(LinkedList<Sample> values, long cutoff)
    {
        while (values.First is { } first && first.Value.EventTime < cutoff) values.RemoveFirst();
    }

    private static void Reset(LinkedList<Sample> values, Sample sample)
    {
        values.Clear();
        values.AddLast(sample);
    }

    private static AlertTrigger Trigger(
        MarketAlertRule rule, PricePoint current, Sample baseline, TriggerDirection direction, decimal change) =>
        new(rule.Id, current.Symbol, AlertRuleKind.MarketPercentage, direction, change,
            rule.ThresholdText, rule.WindowMinutes, null, current.PriceText, baseline.PriceText, current.EventTime);
}
