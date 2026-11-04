using System.Collections.Concurrent;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;

namespace PriceReminder.Windows;

internal sealed class MonitorService : IDisposable
{
    private readonly LocalStore store;
    private readonly ApiClient api;
    private readonly PriceBuffer buffer = new();
    private readonly MarketScanner marketScanner = new();
    private readonly ConcurrentDictionary<string, PricePoint> pendingPrices = new(StringComparer.OrdinalIgnoreCase);
    private readonly Dictionary<string, PricePoint> latestPrices = new(StringComparer.OrdinalIgnoreCase);
    private readonly object gate = new();
    private CancellationTokenSource? serviceCancellation;
    private CancellationTokenSource? connectionCancellation;
    private CancellationTokenSource? marketCancellation;
    private Task? monitorTask;
    private Task? processorTask;
    private string subscriptionSignature = "";
    private bool subscriptionRestart;
    private readonly ConcurrentDictionary<string, byte> initialSymbols = new();
    private long relayVersion;
    private Dictionary<Guid,string> marketRuleKeys = new();
    private HashSet<string> configuredSymbols = new();
    private readonly HashSet<string> relayEventIds = new();
    private bool HasMarketRules { get { lock(store.State) return store.State.MarketRules.Any(r=>r.Enabled); } }
    private string SubscriptionSignature() => HasMarketRules ? "all" : string.Join(",", DesiredSymbols().Where(s=>s!=GoldQuote.Symbol).Order());
    private Task? goldTask;
    public GoldQuote? Gold { get; private set; }
    public string GoldStatus { get; private set; } = "等待黄金报价";
    private HashSet<string> staleSymbols = new(StringComparer.OrdinalIgnoreCase);
    private ConnectionSource source = ConnectionSource.Direct;
    private string message = "监控未启动";
    private bool connected;
    private bool warmingUp;
    private DateTimeOffset? lastReceivedAt;
    private int reconnectCount;
    private string? lastError;
    private int subscribedCount;
    private string marketMessage = "全市场扫描未启用";
    private int marketContractCount;
    private DateTimeOffset? lastMarketReceivedAt;

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

        goldTask = Task.Run(() => GoldLoopAsync(serviceCancellation.Token));
    }

    public void Stop()
    {
        serviceCancellation?.Cancel();
        connectionCancellation?.Cancel();
        marketCancellation?.Cancel();
        marketScanner.ResetAll();
        lock (gate) { marketMessage = "全市场扫描已停止"; marketContractCount = 0; }
        serviceCancellation?.Dispose();
        serviceCancellation = null;
        connectionCancellation = null;
        PersistPrices();
        SetStatus(false, false, "监控已停止");
    }

    public void SubscriptionsChanged()
    {
        lock(store.State) {
            var keys=store.State.MarketRules.Where(r=>r.Enabled).ToDictionary(r=>r.Id,r=>$"{r.WindowMinutes}:{r.ThresholdText}");
            marketScanner.RetainRules(keys.Keys.Where(id=>marketRuleKeys.TryGetValue(id,out var old)&&old==keys[id]).ToHashSet());
            marketRuleKeys=keys;
            var symbols=DesiredSymbols().ToHashSet();
            foreach(var symbol in symbols.Except(configuredSymbols)){buffer.Remove(symbol);initialSymbols[symbol]=0;}
            configuredSymbols=symbols;
        }
        var next = SubscriptionSignature();
        if (next == subscriptionSignature) return;
        subscriptionRestart = true;
        connectionCancellation?.Cancel();
    }

    public void InitializeRule(AlertRule rule)
    {
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
                lastReceivedAt, reconnectCount, lastError, subscribedCount,
                marketMessage, marketContractCount, lastMarketReceivedAt);
        }
    }

    private async Task GoldLoopAsync(CancellationToken token) {
        long last = 0;
        bool initialize = true;
        buffer.Remove(GoldQuote.Symbol);
        while (!token.IsCancellationRequested) {
            try {
                if (!DesiredSymbols().Contains(GoldQuote.Symbol)) { initialize = true; buffer.Remove(GoldQuote.Symbol); }
                else {
                    var quote = await api.GoldQuoteAsync(token);
                    Gold = quote;
                    if (DesiredSymbols().All(s => s == GoldQuote.Symbol)) SetStatus(quote.Eligible, false, "黄金服务端行情");
                    GoldStatus = quote.Eligible ? "实时价格" : (quote.State == "closed" ? "黄金休市" : quote.State == "calendar_unknown" ? "交易日历待更新，提醒已暂停" : "报价已陈旧，提醒已暂停");
                    var point = new PricePoint(GoldQuote.Symbol, quote.Price, quote.EventTime);
                    lock (gate) latestPrices[point.Symbol] = point;
                    if (!quote.Eligible) { initialize = true; buffer.Remove(point.Symbol); }
                    else if (point.EventTime > last) {
                        buffer.Add(point);
                        if (initialize) {
                            lock (store.State) foreach (var rule in store.State.Rules.Where(r => r.Symbol == point.Symbol)) RuleEngine.Initialize(rule,point,buffer);
                            initialize = false;
                        } else Evaluate(point);
                        last = point.EventTime;
                    }
                }
            } catch (OperationCanceledException) when (token.IsCancellationRequested) { return; }
            catch { if (Gold is not null) Gold = Gold with { State = "stale" }; GoldStatus = "黄金行情连接失败，提醒已暂停"; initialize = true; buffer.Remove(GoldQuote.Symbol); }
            PublishState();
            try { await Task.Delay(5000,token); } catch (OperationCanceledException) { return; }
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
                var symbols = DesiredSymbols().Where(s => s != GoldQuote.Symbol).ToArray();
                if(symbols.Length>50){SetStatus(false,false,"最多订阅 50 个普通合约");await Task.Delay(1000,cancellationToken);continue;}
                subscriptionSignature = SubscriptionSignature();
                pendingPrices.Clear();
                marketScanner.ResetAll();
                initialSymbols.Clear();
                foreach (var symbol in symbols) { buffer.Remove(symbol); initialSymbols[symbol] = 0; }
                if (symbols.Length == 0 && !HasMarketRules) { await Task.Delay(1000, cancellationToken); continue; }
                lock (gate) subscribedCount = symbols.Length;
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

            if (subscriptionRestart) { subscriptionRestart = false; continue; }
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
        var all = HasMarketRules;
        var requested = all ? (await api.BinancePerpetualContractsAsync(cancellationToken)).Select(c=>c.Symbol).ToArray() : symbols.ToArray();
        lock(gate) { marketContractCount = all ? requested.Length : 0; marketMessage = all ? "本地全市场扫描 · 正在积累窗口" : "全市场扫描未启用"; }
        using var group = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        async Task ConsumeChunk(string[] chunk) {
            using var socket = api.CreateDirectSocket();
            await socket.ConnectAsync(api.DirectStreamUri(chunk),group.Token);
            SetStatus(true, IsWarmingUp(), "直连币安 · 实时监控中");
            await ReceiveMessagesAsync(socket, json => {
                if(group.IsCancellationRequested)return;
                var point = ApiClient.DecodeDirectPrice(json); if(point is null)return;
                lock(gate) {lastReceivedAt=DateTimeOffset.UtcNow;lastError=null; if(all)lastMarketReceivedAt=lastReceivedAt;}
                pendingPrices.AddOrUpdate(point.Symbol,point,(_,old)=>point.EventTime>old.EventTime?point:old);
            },group.Token);
        }
        var tasks=requested.Chunk(150).Select(ConsumeChunk).ToArray();
        try { await await Task.WhenAny(tasks); } finally { group.Cancel(); try{await Task.WhenAll(tasks);}catch{} }
    }

    private string RelayConfiguration(long version) {
        lock(store.State) return JsonSerializer.Serialize(new {type="configure",protocol=2,version,
            symbols=DesiredSymbols().Where(s=>s!=GoldQuote.Symbol).Order().ToArray(),
            marketRules=store.State.MarketRules.Where(r=>r.Enabled).OrderBy(r=>r.Id).Select(r=>new {id=r.Id,windowMinutes=r.WindowMinutes,thresholdText=r.ThresholdText,isEnabled=true}).ToArray()});
    }

    private async Task ConsumeServerAsync(IReadOnlyList<string> symbols, CancellationToken cancellationToken)
    {
        var relay=await api.CreateServerSocketAsync(symbols,cancellationToken);
        using var socket=relay.Socket;
        await socket.ConnectAsync(relay.Uri,cancellationToken);
        using var session=CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        var lastConfig="";
        var retryAt=DateTimeOffset.UtcNow.AddMinutes(5);
        var sender=Task.Run(async()=>{
            while(!session.IsCancellationRequested) {
                if(DateTimeOffset.UtcNow>=retryAt) {
                    retryAt=DateTimeOffset.UtcNow.AddMinutes(5);
                    using var probe=api.CreateDirectSocket();
                    using var timeout=CancellationTokenSource.CreateLinkedTokenSource(session.Token);
                    timeout.CancelAfter(TimeSpan.FromSeconds(8));
                    try {
                        await probe.ConnectAsync(new Uri("wss://fstream.binance.com/ws"),timeout.Token);
                        throw new DirectAvailableException();
                    } catch(DirectAvailableException){throw;} catch when(!session.IsCancellationRequested){}
                }
                var signature=RelayConfiguration(0);
                if(signature!=lastConfig) {
                    var version=Interlocked.Increment(ref relayVersion);
                    await socket.SendAsync(Encoding.UTF8.GetBytes(RelayConfiguration(version)),WebSocketMessageType.Text,true,session.Token);
                    lastConfig=signature;
                }
                await Task.Delay(1000,session.Token);
            }
        },session.Token);
        var receiver=ReceiveMessagesAsync(socket,json=>{
            if(session.IsCancellationRequested)return;
            var batch=TradeRelayBatch.Parse(json);
            if(batch.Protocol!=2)throw new InvalidDataException("服务器不支持行情协议 v2");
            if(batch.Version!=Interlocked.Read(ref relayVersion))return;
            lock(gate) {lastReceivedAt=DateTimeOffset.UtcNow;lastError=null;marketMessage=HasMarketRules?"服务端全市场扫描中":"全市场扫描未启用";}
            if(HasMarketRules && batch.LatestEventTime>0 && DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()-batch.LatestEventTime>60000)throw new TimeoutException("服务端行情停滞");
            SetStatus(true,IsWarmingUp(),"服务端币安行情 · 实时监控中");
            foreach(var p in batch.Prices)pendingPrices[p.Symbol]=new PricePoint(p.Symbol,p.Price,p.EventTime);
            if(batch.EventId is not null && relayEventIds.Add(batch.EventId)) {
                var alerts=batch.Triggers.Select(t=>t.Convert()).Where(t=>{lock(store.State)return store.State.MarketRules.Any(r=>r.Enabled&&r.Id==t.RuleId);}).ToArray();
                RecordMarketAlerts(alerts);
                if(relayEventIds.Count>2000){relayEventIds.Clear();relayEventIds.Add(batch.EventId);}
            }
        },session.Token);
        try{await await Task.WhenAny(sender,receiver);}finally{session.Cancel();try{await Task.WhenAll(sender,receiver);}catch{}}
    }

    private sealed class DirectAvailableException : Exception {}

    private void RecordMarketAlerts(IReadOnlyList<AlertTrigger> triggers) {
        if(triggers.Count==0)return;
        lock(store.State) {
            foreach(var t in triggers)store.State.History.Insert(0,new TriggerHistory($"{t.RuleId}:{t.Direction}:{t.EventTime}",t.Symbol,t.Kind,t.Direction,t.ChangePercent,t.WindowMinutes,t.ThresholdText,null,t.PriceText,t.EventTime));
            store.Save();
        }
        foreach(var group in triggers.GroupBy(t=>(t.Symbol,t.EventTime)))Triggered?.Invoke(group.ToArray());
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
                if (!pendingPrices.TryRemove(entry.Key, out var point)) continue;
                if(source==ConnectionSource.Direct && HasMarketRules) {
                    AlertTrigger[] alerts;
                    lock(store.State) alerts=store.State.MarketRules.Where(r=>r.Enabled).SelectMany(r=>marketScanner.Evaluate(r,point)).ToArray();
                    RecordMarketAlerts(alerts);
                }
                if(!DesiredSymbols().Contains(point.Symbol) || !buffer.Add(point))continue;
                if(initialSymbols.TryRemove(point.Symbol,out _)) {
                    lock(store.State) foreach(var rule in store.State.Rules.Where(r=>r.Symbol==point.Symbol))RuleEngine.Initialize(rule,point,buffer);
                }
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
                .Distinct(StringComparer.OrdinalIgnoreCase).ToArray();
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
            staleSymbols = latestPrices.Values.Where(point => point.Symbol == GoldQuote.Symbol ? Gold?.Eligible != true : point.EventTime < cutoff)
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
        marketCancellation?.Dispose();
    }
}
