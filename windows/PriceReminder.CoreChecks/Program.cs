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
            EntryPricesValidateCalculateAndPersist();
            Console.WriteLine("PriceReminder Windows core checks passed");
            return 0;
        }
        catch (Exception error)
        {
            Console.Error.WriteLine(error.Message);
            return 1;
        }
    }

    private static void EntryPricesValidateCalculateAndPersist()
    {
        Expect(EntryPriceCalculator.Normalize(" 100.0000 ") == "100.0000", "entry price must normalize input");
        foreach (var invalid in new[] { "", "abc", "0", "-1" })
        {
            try
            {
                EntryPriceCalculator.Normalize(invalid);
                throw new InvalidOperationException($"invalid entry price {invalid} must be rejected");
            }
            catch (FormatException) { }
        }
        Expect(EntryPriceCalculator.Change("104.265", "100", PositionSide.Long).PercentageText == "+4.27%", "long gain must round");
        Expect(EntryPriceCalculator.Change("95.735", "100", PositionSide.Long).PercentageText == "-4.27%", "long loss must keep its sign");
        Expect(EntryPriceCalculator.Change("95.735", "100", PositionSide.Short).PercentageText == "+4.27%", "short must gain when price falls");
        Expect(EntryPriceCalculator.Change("104.265", "100", PositionSide.Short).PercentageText == "-4.27%", "short must lose when price rises");
        Expect(EntryPriceCalculator.Change("99.999999", "100", PositionSide.Long).PercentageText == "0.00%", "negative zero must normalize");
        Expect(!EntryPriceCalculator.IsStale(1_000, 31_000), "exactly 30 seconds must remain fresh");
        Expect(EntryPriceCalculator.IsStale(1_000, 31_001), "prices older than 30 seconds must be stale");

        var state = new PersistedState();
        state.EntryPrices["BTCUSDT"] = "100";
        state.EntryPrices["BTCUSDT"] = "101";
        state.EntryPriceSides["BTCUSDT"] = PositionSide.Short;
        state.TraySymbols.Clear();
        var json = System.Text.Json.JsonSerializer.Serialize(state);
        var restored = System.Text.Json.JsonSerializer.Deserialize<PersistedState>(json)!;
        Expect(restored.EntryPrices["BTCUSDT"] == "101", "entry price must overwrite and survive restart");
        Expect(restored.EntryPriceSides["BTCUSDT"] == PositionSide.Short, "entry side must survive restart");
        var legacy = System.Text.Json.JsonSerializer.Deserialize<PersistedState>(
            "{\"EntryPrices\":{\"ETHUSDT\":\"2000\"}}")!;
        Expect(legacy.EntryPriceSides.GetValueOrDefault("ETHUSDT", PositionSide.Long) == PositionSide.Long,
            "entry prices without a side must migrate to long");
        restored.EntryPrices.Remove("BTCUSDT");
        restored.EntryPriceSides.Remove("BTCUSDT");
        Expect(restored.EntryPrices.Count == 0, "entry price must clear only when explicitly removed");
        Expect(restored.EntryPriceSides.Count == 0, "entry side must clear with its price");
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
