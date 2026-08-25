using System.Collections.Concurrent;
using System.Net.WebSockets;
using System.Text;

namespace PriceReminder.Windows;

internal sealed class MonitorService : IDisposable
{
    private readonly LocalStore store;
    private readonly ApiClient api;
    private readonly PriceBuffer buffer = new();
    private readonly ConcurrentDictionary<string, PricePoint> pendingPrices = new(StringComparer.OrdinalIgnoreCase);
    private readonly Dictionary<string, PricePoint> latestPrices = new(StringComparer.OrdinalIgnoreCase);
    private readonly object gate = new();
    private CancellationTokenSource? serviceCancellation;
    private CancellationTokenSource? connectionCancellation;
    private Task? monitorTask;
    private Task? processorTask;
    private HashSet<string> staleSymbols = new(StringComparer.OrdinalIgnoreCase);
    private ConnectionSource source = ConnectionSource.Direct;
    private string message = "监控未启动";
    private bool connected;
    private bool warmingUp;
    private DateTimeOffset? lastReceivedAt;
    private int reconnectCount;
    private string? lastError;
    private int subscribedCount;

    public event Action<MonitorSnapshot>? StateChanged;
    public event Action<IReadOnlyList<AlertTrigger>>? Triggered;

    public MonitorService(LocalStore store)
    {
        this.store = store;
        api = new ApiClient(store);
        buffer.Restore(store.State.Prices.Values.SelectMany(points => points));
        foreach (var symbol in buffer.Symbols)
        {
            if (buffer.Latest(symbol) is { } point) latestPrices[symbol] = point;
        }
    }

    public bool Running => serviceCancellation is not null;

    public async Task<IReadOnlyList<Contract>> LoadContractsAsync(CancellationToken cancellationToken) =>
        await api.ContractsAsync(cancellationToken);

    public void Start()
    {
        if (serviceCancellation is not null) return;
        serviceCancellation = new CancellationTokenSource();
        source = ConnectionSource.Direct;
        SetStatus(false, true, "正在直连币安");
        monitorTask = Task.Run(() => MonitorLoopAsync(serviceCancellation.Token));
        processorTask = Task.Run(() => ProcessPricesAsync(serviceCancellation.Token));
    }

    public void Stop()
    {
        serviceCancellation?.Cancel();
        connectionCancellation?.Cancel();
        serviceCancellation?.Dispose();
        serviceCancellation = null;
        connectionCancellation = null;
        PersistPrices();
        SetStatus(false, false, "监控已停止");
    }

    public void SubscriptionsChanged() => connectionCancellation?.Cancel();

    public void InitializeRule(AlertRule rule)
    {
        if (rule.Kind != AlertRuleKind.Target) return;
        var current = buffer.Latest(rule.Symbol);
        if (current is not null) RuleEngine.Initialize(rule, current, buffer);
    }

    public MonitorSnapshot Snapshot()
    {
        lock (gate)
        {
            return new MonitorSnapshot(Running, connected, warmingUp, source, message,
                new Dictionary<string, PricePoint>(latestPrices, StringComparer.OrdinalIgnoreCase),
                new HashSet<string>(staleSymbols, StringComparer.OrdinalIgnoreCase),
                lastReceivedAt, reconnectCount, lastError, subscribedCount);
        }
    }

    private async Task MonitorLoopAsync(CancellationToken cancellationToken)
    {
        while (!cancellationToken.IsCancellationRequested)
        {
            connectionCancellation?.Dispose();
            connectionCancellation = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
            try
            {
                var symbols = DesiredSymbols();
                lock (gate) subscribedCount = symbols.Count;
                if (source == ConnectionSource.Direct)
                {
                    SetStatus(false, true, "正在直连币安");
                    await ConsumeDirectAsync(symbols, connectionCancellation.Token);
                }
                else
                {
                    SetStatus(false, true, "正在连接服务端行情");
                    await ConsumeServerAsync(symbols, connectionCancellation.Token);
                }
            }
            catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
            {
                return;
            }
            catch (OperationCanceledException)
            {
                // Subscription changes and direct retries deliberately recycle the socket.
            }
            catch (Exception error)
            {
                lock (gate)
                {
                    reconnectCount++;
                    lastError = ShortError(error);
                }
                SetStatus(false, true, source == ConnectionSource.Direct
                    ? $"币安直连失败，正在切换服务端：{ShortError(error)}"
                    : $"服务端连接中断，5 秒后重新检测币安：{ShortError(error)}");
            }

            source = source == ConnectionSource.Direct ? ConnectionSource.Server : ConnectionSource.Direct;
            PublishState();
            if (source == ConnectionSource.Direct)
            {
                try { await Task.Delay(TimeSpan.FromSeconds(5), cancellationToken); }
                catch (OperationCanceledException) { return; }
            }
        }
    }

    private async Task ConsumeDirectAsync(IReadOnlyList<string> symbols, CancellationToken cancellationToken)
    {
        using var socket = api.CreateDirectSocket();
        using var directTimeout = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        directTimeout.CancelAfter(TimeSpan.FromSeconds(3));
        try
        {
            await socket.ConnectAsync(api.DirectStreamUri(symbols), directTimeout.Token);
        }
        catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
        {
            throw new TimeoutException("3 秒内未连接成功");
        }
        var lastValidPriceAt = DateTimeOffset.UtcNow;
        SetStatus(true, IsWarmingUp(), "直连币安 · 实时监控中");
        using var silence = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        var watchdog = Task.Run(async () =>
        {
            while (!silence.Token.IsCancellationRequested)
            {
                await Task.Delay(TimeSpan.FromSeconds(1), silence.Token);
                if (DateTimeOffset.UtcNow - lastValidPriceAt >= TimeSpan.FromSeconds(3))
                    throw new TimeoutException("3 秒内未收到有效行情");
            }
        }, silence.Token);
        var receiver = ReceiveMessagesAsync(socket, json =>
        {
            var point = ApiClient.DecodeDirectPrice(json);
            if (point is null) return;
            lastValidPriceAt = DateTimeOffset.UtcNow;
            lock (gate)
            {
                lastReceivedAt = lastValidPriceAt;
                lastError = null;
            }
            pendingPrices[point.Symbol] = point;
        }, cancellationToken);

        var completed = await Task.WhenAny(receiver, watchdog);
        silence.Cancel();
        await completed;
        if (completed == receiver) await receiver;
    }

    private async Task ConsumeServerAsync(IReadOnlyList<string> symbols, CancellationToken cancellationToken)
    {
        var relay = await api.CreateServerSocketAsync(symbols, cancellationToken);
        using var socket = relay.Socket;
        await socket.ConnectAsync(relay.Uri, cancellationToken);
        var resume = ApiClient.ResumeMessage(symbols.ToDictionary(
            symbol => symbol, symbol => buffer.Latest(symbol)?.EventTime ?? 0L, StringComparer.OrdinalIgnoreCase));
        await socket.SendAsync(Encoding.UTF8.GetBytes(resume), WebSocketMessageType.Text, true, cancellationToken);
        var connectedAt = DateTimeOffset.UtcNow;
        await ReceiveMessagesAsync(socket, json =>
        {
            var envelope = ApiClient.DecodeRelay(json);
            switch (envelope.Type)
            {
                case "ready": SetStatus(true, IsWarmingUp(), "服务端币安行情 · 实时监控中"); break;
                case "status": SetStatus(true, true, "服务端币安行情 · 正在补齐价格"); break;
                case "price" when envelope.Symbol is not null && envelope.Price is not null && envelope.EventTime is not null:
                    lock (gate)
                    {
                        lastReceivedAt = DateTimeOffset.UtcNow;
                        lastError = null;
                    }
                    pendingPrices[envelope.Symbol] = new PricePoint(
                        envelope.Symbol, envelope.Price, envelope.EventTime.Value, envelope.Replay);
                    break;
            }
            if (DateTimeOffset.UtcNow - connectedAt >= TimeSpan.FromMinutes(5))
                connectionCancellation?.Cancel();
        }, cancellationToken);
    }

    private static async Task ReceiveMessagesAsync(
        ClientWebSocket socket, Action<string> receive, CancellationToken cancellationToken)
    {
        var buffer = new byte[64 * 1024];
        using var message = new MemoryStream();
        while (socket.State == WebSocketState.Open && !cancellationToken.IsCancellationRequested)
        {
            var result = await socket.ReceiveAsync(buffer, cancellationToken);
            if (result.MessageType == WebSocketMessageType.Close)
                throw new WebSocketException($"连接关闭：{result.CloseStatus} {result.CloseStatusDescription}");
            message.Write(buffer, 0, result.Count);
            if (!result.EndOfMessage) continue;
            if (result.MessageType == WebSocketMessageType.Text)
                receive(Encoding.UTF8.GetString(message.GetBuffer(), 0, checked((int)message.Length)));
            message.SetLength(0);
        }
    }

    private async Task ProcessPricesAsync(CancellationToken cancellationToken)
    {
        using var timer = new PeriodicTimer(TimeSpan.FromSeconds(1));
        var persistCounter = 0;
        while (await timer.WaitForNextTickAsync(cancellationToken))
        {
            foreach (var entry in pendingPrices.OrderBy(item => item.Key).ToArray())
            {
                if (!pendingPrices.TryRemove(entry.Key, out var point) || !buffer.Add(point)) continue;
                lock (gate) latestPrices[point.Symbol] = point;
                if (!point.Replay) Evaluate(point);
            }
            UpdateStaleSymbols();
            PublishState();
            if (++persistCounter >= 15)
            {
                persistCounter = 0;
                PersistPrices();
            }
        }
    }

    private void Evaluate(PricePoint point)
    {
        var triggers = new List<AlertTrigger>();
        lock (store.State)
        {
            foreach (var rule in store.State.Rules.Where(rule => rule.Symbol == point.Symbol))
                triggers.AddRange(RuleEngine.Evaluate(rule, point, buffer));
            if (triggers.Count == 0) return;
            foreach (var trigger in triggers)
            {
                store.State.History.Insert(0, new TriggerHistory(
                    $"{trigger.RuleId}:{trigger.Direction}:{trigger.EventTime}", trigger.Symbol,
                    trigger.Kind, trigger.Direction, trigger.ChangePercent, trigger.WindowMinutes,
                    trigger.ThresholdText, trigger.TargetPriceText, trigger.PriceText, trigger.EventTime));
            }
            store.Save();
        }
        Triggered?.Invoke(triggers);
    }

    private IReadOnlyList<string> DesiredSymbols()
    {
        lock (store.State)
        {
            return store.State.Rules.Where(rule => rule.Enabled).Select(rule => rule.Symbol)
                .Concat(store.State.TraySymbols).Append(store.State.PrimarySymbol)
                .Distinct(StringComparer.OrdinalIgnoreCase).Take(50).ToArray();
        }
    }

    private bool IsWarmingUp()
    {
        lock (store.State)
            return store.State.Rules.Any(rule => rule.Enabled && rule.Kind == AlertRuleKind.Percentage &&
                !buffer.Covers(rule.Symbol, rule.WindowMinutes * 60_000L));
    }

    private void UpdateStaleSymbols()
    {
        var cutoff = DateTimeOffset.UtcNow.AddSeconds(-30).ToUnixTimeMilliseconds();
        lock (gate)
            staleSymbols = latestPrices.Values.Where(point => point.EventTime < cutoff)
                .Select(point => point.Symbol).ToHashSet(StringComparer.OrdinalIgnoreCase);
    }

    private void PersistPrices()
    {
        lock (store.State)
        {
            store.State.Prices = buffer.Symbols.ToDictionary(symbol => symbol, buffer.Points, StringComparer.OrdinalIgnoreCase);
            store.Save();
        }
    }

    private void SetStatus(bool isConnected, bool isWarmingUp, string status)
    {
        lock (gate)
        {
            connected = isConnected;
            warmingUp = isWarmingUp;
            message = status;
        }
        PublishState();
    }

    private void PublishState() => StateChanged?.Invoke(Snapshot());

    private static string ShortError(Exception error)
    {
        var value = error.Message.ReplaceLineEndings(" ").Trim();
        return value.Length <= 80 ? value : value[..80] + "…";
    }

    public void Dispose()
    {
        Stop();
        serviceCancellation?.Dispose();
        connectionCancellation?.Dispose();
    }
}
