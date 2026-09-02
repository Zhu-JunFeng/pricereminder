namespace PriceReminder.Windows;

internal static class CoreChecks
{
    private static int Main()
    {
        try
        {
            Expect((int)AlertRuleKind.Percentage == 0 && (int)AlertRuleKind.Target == 1 &&
                (int)AlertRuleKind.MarketPercentage == 2, "persisted rule kind values must remain backward compatible");
            EqualThresholdTriggers();
            DirectionsRearmIndependently();
            MissingWindowDoesNotTrigger();
            ReplayReplacesSameSecond();
            TargetPriceTriggersAndRearms();
            RecentContractsLeadMatchingResultsWithoutChangingTheRest();
            MarketScannerUsesRollingExtremesAndIndependentResets();
            Console.WriteLine("PriceReminder Windows core checks passed");
            return 0;
        }
        catch (Exception error)
        {
            Console.Error.WriteLine(error.Message);
            return 1;
        }
    }

    private static void MarketScannerUsesRollingExtremesAndIndependentResets()
    {
        var scanner = new MarketScanner();
        var rule = new MarketAlertRule { WindowMinutes = 5, ThresholdText = "4" };
        (long At, string Price)[] inputs = [(0, "100"), (30_000, "98"), (60_000, "102"), (61_000, "97"), (62_000, "102")];
        var directions = inputs.SelectMany(item => scanner.Evaluate(rule, new PricePoint("BTCUSDT", item.Price, item.At)))
            .Select(item => item.Direction).ToArray();
        Expect(directions.SequenceEqual([TriggerDirection.Rise, TriggerDirection.Fall, TriggerDirection.Rise]),
            "market directions must use extremes and reset independently");
        var sparseScanner = new MarketScanner();
        var sparseRule = new MarketAlertRule { WindowMinutes = 1, ThresholdText = "5" };
        sparseScanner.Evaluate(sparseRule, new PricePoint("ETHUSDT", "100", 0));
        Expect(sparseScanner.Evaluate(sparseRule, new PricePoint("ETHUSDT", "105", 40_001)).Count == 1,
            "market rule must retain valid sparse prices inside its configured window");
    }

    private static void EqualThresholdTriggers()
    {
        var buffer = new PriceBuffer();
        buffer.Add(new PricePoint("BTCUSDT", "100", 1_000));
        var current = new PricePoint("BTCUSDT", "103", 301_000);
        buffer.Add(current);
        var rule = new AlertRule { Symbol = "BTCUSDT", WindowMinutes = 5, ThresholdText = "3" };
        Expect(RuleEngine.Evaluate(rule, current, buffer).Single().Direction == TriggerDirection.Rise,
            "equal threshold must trigger rise");
    }

    private static void DirectionsRearmIndependently()
    {
        var buffer = new PriceBuffer();
        buffer.Add(new PricePoint("BTCUSDT", "100", 1_000));
        var rule = new AlertRule { Symbol = "BTCUSDT", WindowMinutes = 1, ThresholdText = "2" };
        var rise = new PricePoint("BTCUSDT", "102", 61_000);
        buffer.Add(rise);
        Expect(RuleEngine.Evaluate(rule, rise, buffer).Count == 1, "rise must trigger once");
        var neutral = new PricePoint("BTCUSDT", "100", 62_000);
        buffer.Add(neutral);
        RuleEngine.Evaluate(rule, neutral, buffer);
        var fall = new PricePoint("BTCUSDT", "98", 63_000);
        buffer.Add(fall);
        Expect(RuleEngine.Evaluate(rule, fall, buffer).Single().Direction == TriggerDirection.Fall,
            "fall direction must remain independently armed");
    }

    private static void MissingWindowDoesNotTrigger()
    {
        var buffer = new PriceBuffer();
        var current = new PricePoint("ETHUSDT", "110", 120_000);
        buffer.Add(current);
        var rule = new AlertRule { Symbol = "ETHUSDT", WindowMinutes = 5, ThresholdText = "1" };
        Expect(RuleEngine.Evaluate(rule, current, buffer).Count == 0, "missing baseline must not trigger");
    }

    private static void ReplayReplacesSameSecond()
    {
        var buffer = new PriceBuffer();
        buffer.Add(new PricePoint("SOLUSDT", "100", 1_100));
        buffer.Add(new PricePoint("SOLUSDT", "101", 1_900, true));
        Expect(buffer.Points("SOLUSDT").Count == 1 && buffer.Latest("SOLUSDT")?.PriceText == "101",
            "same-second sample must keep the latest point");
    }

    private static void TargetPriceTriggersAndRearms()
    {
        var buffer = new PriceBuffer();
        var rule = new AlertRule
        {
            Symbol = "BTCUSDT", Kind = AlertRuleKind.Target, WindowMinutes = 0, ThresholdText = "0",
            TargetDirection = TargetDirection.Above, TargetPriceText = "105",
        };
        var current = new PricePoint("BTCUSDT", "104", 1_000);
        buffer.Add(current);
        Expect(RuleEngine.Evaluate(rule, current, buffer).Count == 0, "target should wait below threshold");
        current = new PricePoint("BTCUSDT", "105", 2_000);
        buffer.Add(current);
        Expect(RuleEngine.Evaluate(rule, current, buffer).Single().Kind == AlertRuleKind.Target,
            "equal target should trigger");
        current = new PricePoint("BTCUSDT", "106", 3_000);
        buffer.Add(current);
        Expect(RuleEngine.Evaluate(rule, current, buffer).Count == 0, "target should not repeat");
        current = new PricePoint("BTCUSDT", "104", 4_000);
        buffer.Add(current);
        RuleEngine.Evaluate(rule, current, buffer);
        Expect(!rule.TargetTriggered, "target should rearm after leaving range");
    }

    private static void RecentContractsLeadMatchingResultsWithoutChangingTheRest()
    {
        Contract[] contracts = [
            new("ETHUSDT", "ETH", "USDT", "0.01"), new("BTCUSDT", "BTC", "USDT", "0.10"),
            new("SOLUSDT", "SOL", "USDT", "0.001"), new("BTCDOMUSDT", "BTCDOM", "USDT", "0.10"),
        ];
        Expect(ContractOrdering.Ordered(contracts, ["SOLUSDT", "BTCUSDT"]).Select(item => item.Symbol)
            .SequenceEqual(["SOLUSDT", "BTCUSDT", "ETHUSDT", "BTCDOMUSDT"]), "recent symbols must lead");
        Expect(ContractOrdering.Ordered(contracts, ["SOLUSDT", "BTCDOMUSDT"], "btc").Select(item => item.Symbol)
            .SequenceEqual(["BTCDOMUSDT", "BTCUSDT"]), "only matching recent symbols must lead search");
    }

    private static void Expect(bool condition, string message)
    {
        if (!condition) throw new InvalidOperationException(message);
    }
}
