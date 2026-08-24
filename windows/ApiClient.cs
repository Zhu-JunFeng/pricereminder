using System.Net.Http.Headers;
using System.Net;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace PriceReminder.Windows;

internal sealed class ApiClient
{
    public const string ServerBaseUrl = "https://keyflow.zcn.world/price-reminder";
    private const string BinanceRestUrl = "https://fapi.binance.com/fapi/v1/exchangeInfo";
    private const string BinanceStreamUrl = "wss://fstream.binance.com/stream";
    private static readonly JsonSerializerOptions JsonOptions = new() { PropertyNameCaseInsensitive = true };
    private readonly HttpClient http = new() { Timeout = TimeSpan.FromSeconds(15) };
    private readonly LocalStore store;
    private readonly SemaphoreSlim authenticationGate = new(1, 1);
    private bool tokenValidated;

    public ApiClient(LocalStore store) => this.store = store;

    public async Task<IReadOnlyList<Contract>> ContractsAsync(CancellationToken cancellationToken)
    {
        try
        {
            return await DirectContractsAsync(cancellationToken);
        }
        catch
        {
            return await ServerContractsAsync(cancellationToken);
        }
    }

    public ClientWebSocket CreateDirectSocket()
    {
        var socket = new ClientWebSocket();
        socket.Options.SetRequestHeader("User-Agent", "PriceReminder-Windows");
        socket.Options.KeepAliveInterval = TimeSpan.FromSeconds(20);
        return socket;
    }

    public Uri DirectStreamUri(IEnumerable<string> symbols)
    {
        var streams = string.Join('/', symbols.Distinct(StringComparer.OrdinalIgnoreCase)
            .Order(StringComparer.OrdinalIgnoreCase).Select(symbol => symbol.ToLowerInvariant() + "@trade"));
        if (string.IsNullOrWhiteSpace(streams)) throw new InvalidOperationException("至少选择一个合约");
        return new Uri($"{BinanceStreamUrl}?streams={Uri.EscapeDataString(streams)}");
    }

    public async Task<(ClientWebSocket Socket, Uri Uri)> CreateServerSocketAsync(
        IEnumerable<string> symbols, CancellationToken cancellationToken)
    {
        var token = await AuthenticatedTokenAsync(cancellationToken);
        var subscription = JsonSerializer.Serialize(new
        {
            symbols = symbols.Distinct(StringComparer.OrdinalIgnoreCase).Order(StringComparer.OrdinalIgnoreCase),
        });
        using var request = new HttpRequestMessage(HttpMethod.Put, $"{ServerBaseUrl}/v1/subscriptions")
        {
            Content = new StringContent(subscription, Encoding.UTF8, "application/json"),
        };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        using var response = await http.SendAsync(request, cancellationToken);
        await EnsureSuccessAsync(response, cancellationToken);

        var socket = new ClientWebSocket();
        socket.Options.SetRequestHeader("Authorization", $"Bearer {token}");
        socket.Options.KeepAliveInterval = TimeSpan.FromSeconds(20);
        return (socket, new Uri(ServerBaseUrl.Replace("https://", "wss://", StringComparison.Ordinal) + "/v1/stream"));
    }

    public static string ResumeMessage(IReadOnlyDictionary<string, long> lastEventTimes) =>
        JsonSerializer.Serialize(new { type = "resume", lastEventTime = lastEventTimes });

    public static PricePoint? DecodeDirectPrice(string json)
    {
        using var document = JsonDocument.Parse(json);
        var data = document.RootElement.TryGetProperty("data", out var nested) ? nested : document.RootElement;
        if (!data.TryGetProperty("e", out var eventType) || eventType.GetString() != "trade") return null;
        var price = data.GetProperty("p").GetString();
        var quantity = data.GetProperty("q").GetString();
        if (string.IsNullOrWhiteSpace(price) || price == "0" || quantity == "0") return null;
        return new PricePoint(data.GetProperty("s").GetString() ?? "", price, data.GetProperty("E").GetInt64());
    }

    public static RelayEnvelope DecodeRelay(string json) =>
        JsonSerializer.Deserialize<RelayEnvelope>(json, JsonOptions) ?? new RelayEnvelope();

    private async Task<IReadOnlyList<Contract>> DirectContractsAsync(CancellationToken cancellationToken)
    {
        using var response = await http.GetAsync(BinanceRestUrl, cancellationToken);
        response.EnsureSuccessStatusCode();
        await using var stream = await response.Content.ReadAsStreamAsync(cancellationToken);
        var exchange = await JsonSerializer.DeserializeAsync<ExchangeInfo>(stream, JsonOptions, cancellationToken)
            ?? throw new InvalidDataException("币安合约列表格式错误");
        return exchange.Symbols
            .Where(item => item.Status == "TRADING" && item.ContractType == "PERPETUAL"
                && (item.QuoteAsset == "USDT" || item.QuoteAsset == "USDC"))
            .Select(item => new Contract(
                item.Symbol, item.BaseAsset, item.QuoteAsset,
                item.Filters.FirstOrDefault(filter => filter.FilterType == "PRICE_FILTER")?.TickSize ?? ""))
            .Where(item => item.TickSize.Length > 0)
            .OrderBy(item => item.Symbol, StringComparer.OrdinalIgnoreCase)
            .ToArray();
    }

    private async Task<IReadOnlyList<Contract>> ServerContractsAsync(CancellationToken cancellationToken)
    {
        var token = await AuthenticatedTokenAsync(cancellationToken);
        using var request = new HttpRequestMessage(HttpMethod.Get, $"{ServerBaseUrl}/v1/contracts");
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        using var response = await http.SendAsync(request, cancellationToken);
        await EnsureSuccessAsync(response, cancellationToken);
        await using var stream = await response.Content.ReadAsStreamAsync(cancellationToken);
        var payload = await JsonSerializer.DeserializeAsync<ContractsResponse>(stream, JsonOptions, cancellationToken);
        return payload?.Contracts ?? throw new InvalidDataException("服务端合约列表格式错误");
    }

    private async Task<string> AuthenticatedTokenAsync(CancellationToken cancellationToken)
    {
        await authenticationGate.WaitAsync(cancellationToken);
        try
        {
            if (tokenValidated && !string.IsNullOrWhiteSpace(store.State.DeviceToken))
                return store.State.DeviceToken;

            if (!string.IsNullOrWhiteSpace(store.State.DeviceToken))
            {
                using var refresh = new HttpRequestMessage(HttpMethod.Post, $"{ServerBaseUrl}/v1/devices/refresh");
                refresh.Headers.Authorization = new AuthenticationHeaderValue("Bearer", store.State.DeviceToken);
                using var response = await http.SendAsync(refresh, cancellationToken);
                if (response.IsSuccessStatusCode)
                {
                    tokenValidated = true;
                    return store.State.DeviceToken;
                }
                if (response.StatusCode != HttpStatusCode.Unauthorized)
                    await EnsureAuthenticationSuccessAsync(response, cancellationToken);
                store.State.DeviceToken = null;
                store.Save();
            }

            using var registration = new HttpRequestMessage(HttpMethod.Post, $"{ServerBaseUrl}/v1/devices/register")
            {
                Content = new StringContent(
                    JsonSerializer.Serialize(new { platform = "windows", displayName = Environment.MachineName }),
                    Encoding.UTF8, "application/json"),
            };
            using var created = await http.SendAsync(registration, cancellationToken);
            await EnsureAuthenticationSuccessAsync(created, cancellationToken);
            await using var stream = await created.Content.ReadAsStreamAsync(cancellationToken);
            var result = await JsonSerializer.DeserializeAsync<RegistrationResponse>(stream, JsonOptions, cancellationToken)
                ?? throw new InvalidDataException("服务端设备注册格式错误");
            store.State.DeviceToken = result.Token;
            store.Save();
            tokenValidated = true;
            return result.Token;
        }
        finally
        {
            authenticationGate.Release();
        }
    }

    private async Task EnsureAuthenticationSuccessAsync(HttpResponseMessage response, CancellationToken cancellationToken)
    {
        if (response.IsSuccessStatusCode) return;
        var error = await ResponseErrorAsync(response, cancellationToken);
        throw new HttpRequestException(error, null, response.StatusCode);
    }

    private static async Task EnsureSuccessAsync(HttpResponseMessage response, CancellationToken cancellationToken)
    {
        if (!response.IsSuccessStatusCode)
            throw new HttpRequestException(await ResponseErrorAsync(response, cancellationToken), null, response.StatusCode);
    }

    private static async Task<string> ResponseErrorAsync(HttpResponseMessage response, CancellationToken cancellationToken)
    {
        var body = (await response.Content.ReadAsStringAsync(cancellationToken)).Trim();
        return string.IsNullOrWhiteSpace(body)
            ? $"服务端返回 {(int)response.StatusCode} ({response.ReasonPhrase})"
            : $"服务端返回 {(int)response.StatusCode}：{body}";
    }

    private sealed record ContractsResponse([property: JsonPropertyName("contracts")] List<Contract> Contracts);
    private sealed record RegistrationResponse([property: JsonPropertyName("token")] string Token);
    private sealed record ExchangeInfo([property: JsonPropertyName("symbols")] List<ExchangeSymbol> Symbols);
    private sealed record ExchangeSymbol(
        [property: JsonPropertyName("symbol")] string Symbol,
        [property: JsonPropertyName("status")] string Status,
        [property: JsonPropertyName("contractType")] string ContractType,
        [property: JsonPropertyName("baseAsset")] string BaseAsset,
        [property: JsonPropertyName("quoteAsset")] string QuoteAsset,
        [property: JsonPropertyName("filters")] List<ExchangeFilter> Filters);
    private sealed record ExchangeFilter(
        [property: JsonPropertyName("filterType")] string FilterType,
        [property: JsonPropertyName("tickSize")] string? TickSize);
}

internal sealed class RelayEnvelope
{
    [JsonPropertyName("type")] public string Type { get; set; } = "";
    [JsonPropertyName("symbol")] public string? Symbol { get; set; }
    [JsonPropertyName("price")] public string? Price { get; set; }
    [JsonPropertyName("eventTime")] public long? EventTime { get; set; }
    [JsonPropertyName("replay")] public bool Replay { get; set; }
    [JsonPropertyName("state")] public string? State { get; set; }
    [JsonPropertyName("reason")] public string? Reason { get; set; }
}
