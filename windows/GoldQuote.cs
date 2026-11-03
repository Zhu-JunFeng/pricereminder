using System.Globalization;

namespace PriceReminder.Windows;

internal sealed record GoldQuote(string Price, string PreviousClose, long EventTime, long ReceivedAt, string State)
{
    public const string Symbol = "SGE_AU9999";
    public static readonly Contract Contract = new(Symbol, "上海黄金 Au99.99", "CNY", "0.01");
    public bool Eligible => State == "quoted" && DateTimeOffset.UtcNow.ToUnixTimeMilliseconds() - EventTime <= 120_000;
    public string DailyChange => decimal.TryParse(PreviousClose, NumberStyles.Number, CultureInfo.InvariantCulture, out var close) && close > 0 &&
        decimal.TryParse(Price, NumberStyles.Number, CultureInfo.InvariantCulture, out var price)
        ? ((price-close)/close*100).ToString("+0.00;-0.00;0.00",CultureInfo.InvariantCulture)+"%" : "--";
    public static string Normalize(string value) => value.Contains("黄金") || value.Equals("Au99.99", StringComparison.OrdinalIgnoreCase) ? Symbol : value.Trim().ToUpperInvariant();
    public static string Name(string symbol) => symbol == Symbol ? "上海黄金 Au99.99" : symbol;
}
