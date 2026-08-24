using System.Text.Json;

namespace PriceReminder.Windows;

internal sealed class LocalStore
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        WriteIndented = true,
    };

    private readonly string directory = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "PriceReminder");
    private readonly object gate = new();

    public PersistedState State { get; private set; }

    public LocalStore()
    {
        Directory.CreateDirectory(directory);
        State = Load();
        Trim();
    }

    public void Save()
    {
        lock (gate)
        {
            Trim();
            var path = Path.Combine(directory, "state.json");
            var temporary = path + ".tmp";
            File.WriteAllText(temporary, JsonSerializer.Serialize(State, JsonOptions));
            File.Move(temporary, path, true);
        }
    }

    private PersistedState Load()
    {
        try
        {
            var path = Path.Combine(directory, "state.json");
            return File.Exists(path)
                ? JsonSerializer.Deserialize<PersistedState>(File.ReadAllText(path), JsonOptions) ?? new PersistedState()
                : new PersistedState();
        }
        catch
        {
            return new PersistedState();
        }
    }

    private void Trim()
    {
        var priceCutoff = DateTimeOffset.UtcNow.AddHours(-1).ToUnixTimeMilliseconds();
        foreach (var symbol in State.Prices.Keys.ToList())
        {
            State.Prices[symbol] = State.Prices[symbol].Where(point => point.EventTime >= priceCutoff).ToList();
            if (State.Prices[symbol].Count == 0) State.Prices.Remove(symbol);
        }
        var historyCutoff = DateTimeOffset.UtcNow.AddDays(-30).ToUnixTimeMilliseconds();
        State.History = State.History.Where(item => item.EventTime >= historyCutoff).Take(500).ToList();
        State.TraySymbols = State.TraySymbols.Distinct(StringComparer.OrdinalIgnoreCase).Take(3).ToList();
    }
}
