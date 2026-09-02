namespace PriceReminder.Windows;

internal sealed class TrayApplicationContext : ApplicationContext
{
    private readonly LocalStore store = new();
    private readonly MonitorService monitor;
    private readonly MainForm mainForm;
    private readonly NotifyIcon trayIcon;
    private readonly ToolStripMenuItem monitorItem;
    private readonly ToolStripSeparator priceSeparator = new();
    private bool exiting;

    public TrayApplicationContext()
    {
        monitor = new MonitorService(store);
        mainForm = new MainForm(store, monitor);
        _ = mainForm.Handle;
        monitorItem = new ToolStripMenuItem("停止监控", null, (_, _) => ToggleMonitoring());
        var menu = new ContextMenuStrip();
        menu.Items.Add(new ToolStripMenuItem("打开币价提醒", null, (_, _) => ShowWindow()) { Font = new Font("Segoe UI", 9, FontStyle.Bold) });
        menu.Items.Add(priceSeparator);
        menu.Items.Add(monitorItem);
        menu.Items.Add(new ToolStripMenuItem("退出", null, (_, _) => ExitApplication()));

        trayIcon = new NotifyIcon
        {
            Icon = BrandIcon.Create(),
            Text = "币价提醒 · 正在连接",
            Visible = true,
            ContextMenuStrip = menu,
        };
        trayIcon.DoubleClick += (_, _) => ShowWindow();
        trayIcon.MouseClick += (_, eventArgs) =>
        {
            if (eventArgs.Button == MouseButtons.Left) ShowWindow();
        };

        mainForm.FormClosing += (_, eventArgs) =>
        {
            if (exiting) return;
            eventArgs.Cancel = true;
            mainForm.Hide();
            trayIcon.ShowBalloonTip(2500, "币价提醒仍在运行", "价格监控已收起到 Windows 通知区域。", ToolTipIcon.Info);
        };
        monitor.StateChanged += snapshot => Ui(() => ApplySnapshot(snapshot));
        monitor.Triggered += triggers => Ui(() => ShowTriggers(triggers));
        mainForm.TestNotificationRequested += () => Ui(() =>
            trayIcon.ShowBalloonTip(5000, "币价提醒测试", "系统通知可正常接收", ToolTipIcon.Info));

        monitor.Start();
        _ = LoadContractsAsync();
        if (!Environment.GetCommandLineArgs().Contains("--tray", StringComparer.OrdinalIgnoreCase)) ShowWindow();
    }

    private async Task LoadContractsAsync()
    {
        try
        {
            var contracts = await monitor.LoadContractsAsync(CancellationToken.None);
            Ui(() => mainForm.SetContracts(contracts));
        }
        catch (Exception error)
        {
            Ui(() => mainForm.SetContractsError($"合约列表获取失败：{error.Message}"));
        }
    }

    private void ApplySnapshot(MonitorSnapshot snapshot)
    {
        mainForm.ApplySnapshot(snapshot);
        monitorItem.Text = snapshot.Running ? "停止监控" : "开始监控";
        var values = store.State.TraySymbols.Take(3).Select(symbol =>
            snapshot.Prices.TryGetValue(symbol, out var point) ? $"{ShortSymbol(symbol)} {point.PriceText}" : $"{ShortSymbol(symbol)} --");
        var prices = string.Join("  ·  ", values);
        trayIcon.Text = LimitTooltip(string.IsNullOrWhiteSpace(prices) ? snapshot.Message : prices);

        var menu = trayIcon.ContextMenuStrip!;
        while (menu.Items.IndexOf(priceSeparator) > 1) menu.Items.RemoveAt(1);
        foreach (var symbol in store.State.TraySymbols.Take(3).Reverse<string>())
        {
            var price = snapshot.Prices.TryGetValue(symbol, out var point) ? point.PriceText : "--";
            menu.Items.Insert(1, new ToolStripMenuItem($"{symbol}   {price}") { Enabled = false });
        }
    }

    private void ShowTriggers(IReadOnlyList<AlertTrigger> triggers)
    {
        if (triggers.Count == 0) return;
        var first = triggers[0];
        var body = string.Join("；", triggers.Select(trigger => trigger.Kind == AlertRuleKind.Target
            ? $"{(trigger.Direction == TriggerDirection.Rise ? "达到或高于" : "达到或低于")}目标价 {trigger.TargetPriceText}"
            : $"{trigger.WindowMinutes}分钟{(trigger.Direction == TriggerDirection.Rise ? "上涨" : "下跌")} " +
              $"{(trigger.ChangePercent ?? 0):F2}%（阈值 {trigger.ThresholdText}%）"));
        var title = first.Kind == AlertRuleKind.MarketPercentage ? $"{first.Symbol} 全市场预警" : $"{first.Symbol} 价格预警";
        trayIcon.ShowBalloonTip(8000, title, $"{body} · 最新价 {first.PriceText}",
            first.Direction == TriggerDirection.Rise ? ToolTipIcon.Info : ToolTipIcon.Warning);
        mainForm.RefreshHistory();
    }

    private void ToggleMonitoring()
    {
        if (monitor.Running) monitor.Stop(); else monitor.Start();
    }

    private void ShowWindow()
    {
        mainForm.Show();
        if (mainForm.WindowState == FormWindowState.Minimized) mainForm.WindowState = FormWindowState.Normal;
        mainForm.Activate();
    }

    private void ExitApplication()
    {
        exiting = true;
        monitor.Dispose();
        trayIcon.Visible = false;
        trayIcon.Dispose();
        mainForm.Close();
        ExitThread();
    }

    private void Ui(Action action)
    {
        if (mainForm.IsDisposed) return;
        if (mainForm.InvokeRequired) mainForm.BeginInvoke(action); else action();
    }

    private static string ShortSymbol(string symbol) => symbol.EndsWith("USDT", StringComparison.OrdinalIgnoreCase)
        ? symbol[..^4] : symbol.EndsWith("USDC", StringComparison.OrdinalIgnoreCase) ? symbol[..^4] : symbol;
    private static string LimitTooltip(string text) => text.Length <= 63 ? text : text[..62];
}
