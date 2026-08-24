namespace PriceReminder.Windows;

internal sealed class PriceBuffer
{
    private const long RetentionMilliseconds = 60L * 60L * 1000L;
    private readonly Dictionary<string, List<PricePoint>> points = new(StringComparer.OrdinalIgnoreCase);
    private readonly object gate = new();

    public bool Add(PricePoint point)
    {
        lock (gate)
        {
            if (!points.TryGetValue(point.Symbol, out var values))
            {
                values = [];
                points[point.Symbol] = values;
            }
            if (values.Count > 0)
            {
                var last = values[^1];
                if (point.EventTime < last.EventTime) return false;
                if (point.EventTime / 1000 == last.EventTime / 1000)
                {
                    values[^1] = point;
                    return true;
                }
            }
            values.Add(point);
            var cutoff = point.EventTime - RetentionMilliseconds;
            var removeCount = values.FindIndex(value => value.EventTime >= cutoff);
            if (removeCount < 0) values.Clear();
            else if (removeCount > 0) values.RemoveRange(0, removeCount);
            return true;
        }
    }

    public void Restore(IEnumerable<PricePoint> restored)
    {
        foreach (var point in restored.OrderBy(item => item.EventTime).ThenBy(item => item.Symbol)) Add(point);
    }

    public PricePoint? Latest(string symbol)
    {
        lock (gate) return points.TryGetValue(symbol, out var values) && values.Count > 0 ? values[^1] : null;
    }

    public List<PricePoint> Points(string symbol)
    {
        lock (gate) return points.TryGetValue(symbol, out var values) ? [.. values] : [];
    }

    public IEnumerable<string> Symbols
    {
        get { lock (gate) return points.Keys.ToArray(); }
    }

    public bool Covers(string symbol, long durationMilliseconds)
    {
        lock (gate)
        {
            return points.TryGetValue(symbol, out var values) && values.Count >= 2
                && values[^1].EventTime - values[0].EventTime >= durationMilliseconds;
        }
    }

    public PricePoint? AtOrBefore(string symbol, long eventTime)
    {
        lock (gate)
        {
            if (!points.TryGetValue(symbol, out var values)) return null;
            var low = 0;
            var high = values.Count;
            while (low < high)
            {
                var middle = (low + high) >> 1;
                if (values[middle].EventTime <= eventTime) low = middle + 1;
                else high = middle;
            }
            return low == 0 ? null : values[low - 1];
        }
    }
}

internal static class RuleEngine
{
    private const long StaleMilliseconds = 30_000;

    public static IReadOnlyList<AlertTrigger> Evaluate(AlertRule rule, PricePoint current, PriceBuffer buffer)
    {
        if (!rule.Enabled || !string.Equals(current.Symbol, rule.Symbol, StringComparison.OrdinalIgnoreCase)) return [];
        var cutoff = current.EventTime - rule.WindowMinutes * 60_000L;
        var baseline = buffer.AtOrBefore(rule.Symbol, cutoff);
        if (baseline is null || cutoff - baseline.EventTime > StaleMilliseconds || baseline.Price == 0) return [];
        var change = (current.Price - baseline.Price) / baseline.Price * 100m;
        if (rule.RiseTriggered && change < rule.Threshold) rule.RiseTriggered = false;
        if (rule.FallTriggered && change > -rule.Threshold) rule.FallTriggered = false;

        var triggers = new List<AlertTrigger>(2);
        if (!rule.RiseTriggered && change >= rule.Threshold)
        {
            rule.RiseTriggered = true;
            triggers.Add(MakeTrigger(rule, current, baseline, TriggerDirection.Rise, change));
        }
        if (!rule.FallTriggered && change <= -rule.Threshold)
        {
            rule.FallTriggered = true;
            triggers.Add(MakeTrigger(rule, current, baseline, TriggerDirection.Fall, change));
        }
        return triggers;
    }

    private static AlertTrigger MakeTrigger(
        AlertRule rule, PricePoint current, PricePoint baseline, TriggerDirection direction, decimal change) =>
        new(rule.Id, rule.Symbol, direction, change, rule.ThresholdText, rule.WindowMinutes,
            current.PriceText, baseline.PriceText, current.EventTime);
}
