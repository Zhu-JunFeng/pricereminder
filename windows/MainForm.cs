using System.Globalization;

namespace PriceReminder.Windows;

internal sealed class MainForm : Form
{
    private static readonly Color Primary = Color.FromArgb(0, 127, 136);
    private static readonly Color PrimaryHover = Color.FromArgb(0, 108, 116);
    private static readonly Color PrimarySoft = Color.FromArgb(227, 244, 245);
    private static readonly Color Ink = Color.FromArgb(23, 42, 46);
    private static readonly Color Muted = Color.FromArgb(82, 103, 107);
    private static readonly Color Surface = Color.FromArgb(244, 248, 248);
    private static readonly Color Divider = Color.FromArgb(213, 222, 223);
    private static readonly Color Warning = Color.FromArgb(153, 88, 17);
    private static readonly Color WarningSoft = Color.FromArgb(255, 244, 224);
    private readonly LocalStore store;
    private readonly MonitorService monitor;
    private readonly Label statusLabel = new();
    private readonly Label sourceLabel = new();
    private readonly Label symbolLabel = new();
    private readonly Label priceLabel = new();
    private readonly Label updatedLabel = new();
    private readonly Button monitorButton = new();
    private readonly ComboBox primarySymbol = new();
    private readonly ComboBox ruleSymbol = new();
    private readonly NumericUpDown ruleWindow = new();
    private readonly NumericUpDown ruleThreshold = new();
    private readonly NumericUpDown targetPrice = new();
    private readonly ComboBox ruleKind = new();
    private readonly ComboBox targetDirection = new();
    private readonly ListView rulesList = new();
    private readonly ListView historyList = new();
    private readonly CheckedListBox traySymbols = new();
    private readonly TextBox traySearch = new();
    private readonly Label contractsError = new();
    private readonly CheckBox startupCheck = new();
    private readonly Label diagnosticsLabel = new();
    private Panel? ruleWindowField;
    private Panel? ruleThresholdField;
    private Panel? targetDirectionField;
    private Panel? targetPriceField;
    private readonly Button saveRuleButton = new() { Text = "添加规则", AutoSize = true };
    private readonly Button cancelRuleEditButton = new() { Text = "取消", AutoSize = true, Visible = false };
    private IReadOnlyList<Contract> contracts = [];
    private bool updatingTraySymbols;
    private bool updatingContractCombos;
    private Guid? editingRuleId;
    private Guid? editingMarketRuleId;

    public event Action? TestNotificationRequested;

    public MainForm(LocalStore store, MonitorService monitor)
    {
        this.store = store;
        this.monitor = monitor;
        Text = "币价提醒";
        Icon = BrandIcon.Create();
        StartPosition = FormStartPosition.CenterScreen;
        MinimumSize = new Size(840, 620);
        Size = new Size(940, 700);
        BackColor = Surface;
        Font = new Font("Segoe UI Variable Text", 10);
        DoubleBuffered = true;

        var tabs = new TabControl
        {
            Dock = DockStyle.Fill,
            DrawMode = TabDrawMode.OwnerDrawFixed,
            ItemSize = new Size(132, 42),
            SizeMode = TabSizeMode.Fixed,
            Padding = new Point(18, 8),
        };
        tabs.DrawItem += (_, eventArgs) => DrawTab(tabs, eventArgs);
        tabs.TabPages.Add(CreateMarketPage());
        tabs.TabPages.Add(CreateRulesPage());
        tabs.TabPages.Add(CreateSettingsPage());
        Controls.Add(tabs);
        RefreshRules();
        RefreshHistory();
        startupCheck.Checked = StartupManager.Enabled;
    }

    public void SetContracts(IReadOnlyList<Contract> values)
    {
        contracts = values;
        contractsError.Visible = false;
        RefreshContractCombos();
        UpdateTraySymbolList();
    }

    public void SetContractsError(string message)
    {
        contractsError.Text = message;
        contractsError.Visible = true;
    }

    public void ApplySnapshot(MonitorSnapshot snapshot)
    {
        statusLabel.Text = snapshot.Message;
        sourceLabel.Text = snapshot.Source == ConnectionSource.Direct ? "终端直连" : "服务端中继";
        sourceLabel.ForeColor = snapshot.Connected ? Primary : Warning;
        sourceLabel.BackColor = snapshot.Connected ? PrimarySoft : WarningSoft;
        monitorButton.Text = snapshot.Running ? "停止监控" : "开始监控";
        var symbol = store.State.PrimarySymbol;
        symbolLabel.Text = symbol;
        if (snapshot.Prices.TryGetValue(symbol, out var point))
        {
            priceLabel.Text = point.PriceText;
            updatedLabel.Text = snapshot.StaleSymbols.Contains(symbol)
                ? "价格已陈旧 · 正在重新连接"
                : $"币安最新成交价 · {DateTimeOffset.FromUnixTimeMilliseconds(point.EventTime).ToLocalTime():HH:mm:ss}";
        }
        else
        {
            priceLabel.Text = "--";
            updatedLabel.Text = "等待第一条价格";
        }
        var lastReceived = snapshot.LastReceivedAt?.ToLocalTime().ToString("HH:mm:ss", CultureInfo.InvariantCulture) ?? "尚未收到";
        var lastMarketReceived = snapshot.LastMarketReceivedAt?.ToLocalTime().ToString("HH:mm:ss", CultureInfo.InvariantCulture) ?? "尚未收到";
        var latestEvent = snapshot.Prices.Values.Select(item => item.EventTime).DefaultIfEmpty(0).Max();
        var delay = latestEvent == 0 ? "尚未收到行情" : $"{Math.Max(0, DateTimeOffset.UtcNow.ToUnixTimeMilliseconds() - latestEvent) / 1000.0:F1} 秒";
        diagnosticsLabel.Text =
            $"连接路径：{(snapshot.Source == ConnectionSource.Direct ? "终端直连" : "服务端中继")}\r\n" +
            $"订阅合约：{snapshot.SubscribedCount} 个\r\n行情延迟：{delay}\r\n最后接收：{lastReceived}\r\n" +
            $"重连次数：{snapshot.ReconnectCount}" +
            $"\r\n全市场扫描：{snapshot.MarketMessage}\r\n全市场覆盖：{snapshot.MarketContractCount} 个合约" +
            $"\r\n全市场最后接收：{lastMarketReceived}" +
            (string.IsNullOrWhiteSpace(snapshot.LastError) ? "" : $"\r\n最近错误：{snapshot.LastError}");
    }

    public void RefreshHistory()
    {
        historyList.BeginUpdate();
        historyList.Items.Clear();
        foreach (var item in store.State.History.Take(100))
        {
            var direction = item.Kind == AlertRuleKind.Target
                ? (item.Direction == TriggerDirection.Rise ? "↑ 达到或高于" : "↓ 达到或低于")
                : (item.Direction == TriggerDirection.Rise ? "↑ 上涨" : "↓ 下跌");
            historyList.Items.Add(new ListViewItem(new[] {
                item.Symbol, direction,
                item.Kind == AlertRuleKind.Target ? item.TargetPriceText ?? "--" : $"{(item.ChangePercent ?? 0):F2}%",
                item.PriceText,
                DateTimeOffset.FromUnixTimeMilliseconds(item.EventTime).ToLocalTime().ToString("MM-dd HH:mm", CultureInfo.InvariantCulture),
            }));
        }
        historyList.EndUpdate();
    }

    private TabPage CreateMarketPage()
    {
        var page = Page("行情");
        var layout = new TableLayoutPanel
        {
            Dock = DockStyle.Fill, Padding = new Padding(28, 24, 28, 24), ColumnCount = 2, RowCount = 8,
        };
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 70));
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 30));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));

        var title = TextLabel("实时价格", 22, FontStyle.Bold);
        layout.Controls.Add(title, 0, 0);
        layout.SetColumnSpan(title, 2);
        primarySymbol.DropDownStyle = ComboBoxStyle.DropDown;
        primarySymbol.FlatStyle = FlatStyle.Flat;
        primarySymbol.Margin = new Padding(0, 18, 12, 22);
        primarySymbol.SelectedIndexChanged += (_, _) => SelectPrimary();
        primarySymbol.KeyDown += (_, eventArgs) => { if (eventArgs.KeyCode == Keys.Enter) SelectPrimary(); };
        primarySymbol.DropDown += (_, _) => RefreshContractCombos();
        layout.Controls.Add(primarySymbol, 0, 1);
        sourceLabel.AutoSize = false;
        sourceLabel.Size = new Size(90, 28);
        sourceLabel.Anchor = AnchorStyles.Right;
        sourceLabel.Font = new Font("Segoe UI", 9, FontStyle.Bold);
        sourceLabel.TextAlign = ContentAlignment.MiddleCenter;
        sourceLabel.BackColor = PrimarySoft;
        sourceLabel.ForeColor = Primary;
        layout.Controls.Add(sourceLabel, 1, 1);
        symbolLabel.AutoSize = true;
        symbolLabel.ForeColor = Muted;
        layout.Controls.Add(symbolLabel, 0, 2);
        priceLabel.AutoSize = true;
        priceLabel.Font = new Font("Cascadia Mono", 32, FontStyle.Bold);
        priceLabel.ForeColor = Ink;
        priceLabel.Margin = new Padding(0, 3, 0, 0);
        layout.Controls.Add(priceLabel, 0, 3);
        layout.SetColumnSpan(priceLabel, 2);
        updatedLabel.AutoSize = true;
        updatedLabel.ForeColor = Muted;
        layout.Controls.Add(updatedLabel, 0, 4);
        layout.SetColumnSpan(updatedLabel, 2);

        var divider = new Panel { Height = 1, Dock = DockStyle.Top, BackColor = Divider, Margin = new Padding(0, 12, 0, 16) };
        layout.Controls.Add(divider, 0, 5);
        layout.SetColumnSpan(divider, 2);
        statusLabel.AutoSize = true;
        statusLabel.ForeColor = Ink;
        statusLabel.Font = new Font("Segoe UI", 10, FontStyle.Bold);
        monitorButton.Text = "停止监控";
        monitorButton.AutoSize = true;
        monitorButton.Anchor = AnchorStyles.Right;
        StylePrimaryButton(monitorButton);
        monitorButton.Click += (_, _) => { if (monitor.Running) monitor.Stop(); else monitor.Start(); };
        var statusSurface = new TableLayoutPanel
        {
            Dock = DockStyle.Top, AutoSize = true, BackColor = Surface,
            Padding = new Padding(16, 13, 12, 13), ColumnCount = 2,
        };
        statusSurface.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        statusSurface.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        statusSurface.Controls.Add(statusLabel, 0, 0);
        statusSurface.Controls.Add(monitorButton, 1, 0);
        layout.Controls.Add(statusSurface, 0, 6);
        layout.SetColumnSpan(statusSurface, 2);
        contractsError.AutoSize = true;
        contractsError.ForeColor = Color.FromArgb(198, 62, 62);
        contractsError.Visible = false;
        contractsError.Margin = new Padding(0, 14, 0, 0);
        layout.Controls.Add(contractsError, 0, 7);
        layout.SetColumnSpan(contractsError, 2);
        page.Controls.Add(layout);
        return page;
    }

    private TabPage CreateRulesPage()
    {
        var page = Page("预警");
        var layout = new TableLayoutPanel
        {
            Dock = DockStyle.Fill, Padding = new Padding(24), ColumnCount = 1, RowCount = 4,
        };
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent, 55));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent, 45));
        layout.Controls.Add(TextLabel("价格预警", 20, FontStyle.Bold), 0, 0);

        var addRow = new FlowLayoutPanel
        {
            Dock = DockStyle.Fill, AutoSize = true, BackColor = Surface,
            Padding = new Padding(14, 12, 14, 8), Margin = new Padding(0, 14, 0, 14),
            WrapContents = false,
        };
        ruleSymbol.Width = 180;
        ruleSymbol.DropDownStyle = ComboBoxStyle.DropDown;
        ruleWindow.Minimum = 1; ruleWindow.Maximum = 60; ruleWindow.Value = 5; ruleWindow.Width = 72;
        ruleThreshold.Minimum = .1m; ruleThreshold.Maximum = 100; ruleThreshold.DecimalPlaces = 1; ruleThreshold.Increment = .1m; ruleThreshold.Value = 3; ruleThreshold.Width = 82;
        targetPrice.Minimum = .00000001m; targetPrice.Maximum = 1000000000; targetPrice.DecimalPlaces = 8; targetPrice.Increment = 1; targetPrice.Width = 118;
        ruleKind.DropDownStyle = ComboBoxStyle.DropDownList;
        ConfigureRuleKinds("单合约", "全市场", "目标价格");
        ruleKind.Width = 92;
        targetDirection.DropDownStyle = ComboBoxStyle.DropDownList;
        targetDirection.Items.AddRange(["达到或高于", "达到或低于"]);
        targetDirection.SelectedIndex = 0;
        targetDirection.Width = 112;
        StylePrimaryButton(saveRuleButton);
        saveRuleButton.Click += (_, _) => SaveRule();
        StyleSecondaryButton(cancelRuleEditButton);
        cancelRuleEditButton.Click += (_, _) => CancelRuleEditing();
        ruleWindowField = Field("分钟", ruleWindow);
        ruleThresholdField = Field("变化 %", ruleThreshold);
        targetDirectionField = Field("条件", targetDirection);
        targetPriceField = Field("目标价格", targetPrice);
        addRow.Controls.AddRange(new Control[] {
            Field("类型", ruleKind), Field("合约", ruleSymbol), ruleWindowField, ruleThresholdField,
            targetDirectionField, targetPriceField, saveRuleButton, cancelRuleEditButton,
        });
        ruleKind.SelectedIndexChanged += (_, _) => UpdateRuleFields();
        ruleSymbol.SelectionChangeCommitted += (_, _) =>
        {
            var symbol = ruleSymbol.Text.Trim().ToUpperInvariant();
            if (contracts.Any(item => item.Symbol == symbol)) store.RecordRecentSymbol(symbol);
        };
        ruleSymbol.DropDown += (_, _) => RefreshContractCombos();
        UpdateRuleFields();
        layout.Controls.Add(addRow, 0, 1);

        ConfigureList(rulesList, ["合约", "规则", "状态"], [145, 520, 100]);
        rulesList.SmallImageList = new ImageList { ImageSize = new Size(16, 16), ColorDepth = ColorDepth.Depth32Bit };
        rulesList.SmallImageList.Images.Add(CreateEditIcon());
        rulesList.MouseClick += (_, eventArgs) =>
        {
            var item = rulesList.GetItemAt(eventArgs.X, eventArgs.Y);
            if (item is not null && eventArgs.X - item.Bounds.Left <= 28) BeginRuleEditing(item);
        };
        rulesList.DoubleClick += (_, _) => ToggleSelectedRule();
        rulesList.KeyDown += (_, eventArgs) =>
        {
            if (eventArgs.KeyCode == Keys.Delete) DeleteSelectedRule();
            if (eventArgs.KeyCode == Keys.Space) ToggleSelectedRule();
        };
        var rulesMenu = new ContextMenuStrip();
        rulesMenu.Items.Add("启用 / 暂停", null, (_, _) => ToggleSelectedRule());
        rulesMenu.Items.Add("编辑", null, (_, _) => BeginSelectedRuleEditing());
        rulesMenu.Items.Add("删除", null, (_, _) => DeleteSelectedRule());
        rulesList.ContextMenuStrip = rulesMenu;
        layout.Controls.Add(ListSection("已配置规则", rulesList), 0, 2);

        ConfigureList(historyList, ["合约", "方向", "目标 / 变化", "触发价格", "时间"], [130, 120, 125, 210, 150]);
        layout.Controls.Add(ListSection("最近触发", historyList, new Padding(0, 16, 0, 0)), 0, 3);
        page.Controls.Add(layout);
        return page;
    }

    private TabPage CreateSettingsPage()
    {
        var page = Page("设置");
        var layout = new TableLayoutPanel
        {
            Dock = DockStyle.Fill, Padding = new Padding(28, 24, 28, 24), ColumnCount = 2, RowCount = 5,
        };
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 54));
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 46));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        var title = TextLabel("任务栏显示", 20, FontStyle.Bold);
        layout.Controls.Add(title, 0, 0);
        layout.SetColumnSpan(title, 2);
        var hint = TextLabel("选择最多三个合约；悬停托盘图标可查看最新价格。", 9, FontStyle.Regular);
        hint.ForeColor = Muted;
        hint.Margin = new Padding(0, 4, 0, 16);
        layout.Controls.Add(hint, 0, 1);
        layout.SetColumnSpan(hint, 2);
        traySearch.PlaceholderText = "输入合约代码搜索";
        traySearch.Dock = DockStyle.Fill;
        traySearch.Margin = new Padding(0, 0, 0, 12);
        traySearch.TextChanged += (_, _) => UpdateTraySymbolList();
        layout.Controls.Add(traySearch, 0, 2);
        traySymbols.Dock = DockStyle.Fill;
        traySymbols.CheckOnClick = true;
        traySymbols.BorderStyle = BorderStyle.None;
        traySymbols.BackColor = Surface;
        traySymbols.ForeColor = Ink;
        traySymbols.Padding = new Padding(8);
        traySymbols.ItemCheck += TraySymbolChecked;
        layout.Controls.Add(traySymbols, 0, 3);

        var details = new Panel { Dock = DockStyle.Fill, BackColor = Surface, Padding = new Padding(18), Margin = new Padding(18, 0, 0, 0) };
        var detailFlow = new FlowLayoutPanel
        {
            Dock = DockStyle.Fill, FlowDirection = FlowDirection.TopDown, WrapContents = false, AutoScroll = true,
        };
        detailFlow.Controls.Add(TextLabel("运行设置", 14, FontStyle.Bold));
        startupCheck.Text = "登录 Windows 后自动启动";
        startupCheck.AutoSize = true;
        startupCheck.Margin = new Padding(0, 16, 0, 10);
        startupCheck.CheckedChanged += (_, _) => StartupManager.Enabled = startupCheck.Checked;
        detailFlow.Controls.Add(startupCheck);
        var note = TextLabel("关闭窗口只会收起到通知区域。选择“退出”才会停止监控。", 9, FontStyle.Regular);
        note.ForeColor = Muted;
        note.MaximumSize = new Size(250, 0);
        detailFlow.Controls.Add(note);
        var diagnosticsTitle = TextLabel("连接自检", 14, FontStyle.Bold);
        diagnosticsTitle.Margin = new Padding(0, 18, 0, 0);
        detailFlow.Controls.Add(diagnosticsTitle);
        diagnosticsLabel.AutoSize = true;
        diagnosticsLabel.ForeColor = Muted;
        diagnosticsLabel.MaximumSize = new Size(280, 0);
        diagnosticsLabel.Margin = new Padding(0, 18, 0, 8);
        detailFlow.Controls.Add(diagnosticsLabel);
        var testButton = new Button { Text = "发送测试通知", AutoSize = true };
        StyleSecondaryButton(testButton);
        testButton.Click += (_, _) => TestNotificationRequested?.Invoke();
        detailFlow.Controls.Add(testButton);
        details.Controls.Add(detailFlow);
        layout.Controls.Add(details, 1, 2);
        layout.SetRowSpan(details, 2);
        var source = TextLabel("行情来源：币安 U 本位永续最新成交价；终端不通时使用服务端转发的同一币安行情。", 9, FontStyle.Regular);
        source.ForeColor = Muted;
        source.Margin = new Padding(0, 18, 0, 0);
        source.MaximumSize = new Size(650, 0);
        layout.Controls.Add(source, 0, 4);
        layout.SetColumnSpan(source, 2);
        page.Controls.Add(layout);
        return page;
    }

    private void SelectPrimary()
    {
        if (updatingContractCombos) return;
        var symbol = primarySymbol.Text.Trim().ToUpperInvariant();
        if (!contracts.Any(item => item.Symbol == symbol)) return;
        store.State.PrimarySymbol = symbol;
        store.RecordRecentSymbol(symbol);
        RefreshContractCombos();
        store.Save();
        monitor.SubscriptionsChanged();
    }

    private void SaveRule()
    {
        var symbol = ruleSymbol.Text.Trim().ToUpperInvariant();
        var isMarket = ruleKind.Text == "全市场";
        if (!isMarket && !contracts.Any(item => item.Symbol == symbol))
        {
            MessageBox.Show(this, "请选择有效的 U 本位永续合约。", "无法添加规则", MessageBoxButtons.OK, MessageBoxIcon.Warning);
            return;
        }
        var isTarget = ruleKind.Text == "目标价格";
        var threshold = ruleThreshold.Value.ToString("0.0", CultureInfo.InvariantCulture);
        var target = targetPrice.Value.ToString("0.########", CultureInfo.InvariantCulture);
        var targetDirectionValue = targetDirection.SelectedIndex == 0 ? TargetDirection.Above : TargetDirection.Below;
        if (isMarket)
        {
            if ((editingMarketRuleId is null && store.State.Rules.Count + store.State.MarketRules.Count >= 50) ||
                store.State.MarketRules.Any(rule => rule.Id != editingMarketRuleId &&
                    rule.WindowMinutes == (int)ruleWindow.Value && rule.ThresholdText == threshold))
            {
                MessageBox.Show(this, "规则已存在，或已达到 50 条上限。", "无法添加规则", MessageBoxButtons.OK, MessageBoxIcon.Warning);
                return;
            }
            var marketRule = new MarketAlertRule
            {
                Id = editingMarketRuleId ?? Guid.NewGuid(), WindowMinutes = (int)ruleWindow.Value,
                ThresholdText = threshold,
            };
            if (editingMarketRuleId is { } marketId)
            {
                var index = store.State.MarketRules.FindIndex(rule => rule.Id == marketId);
                if (index < 0) return;
                marketRule.Enabled = store.State.MarketRules[index].Enabled;
                store.State.MarketRules[index] = marketRule;
            }
            else store.State.MarketRules.Add(marketRule);
            store.Save();
            RefreshRules();
            CancelRuleEditing();
            monitor.SubscriptionsChanged();
            return;
        }
        if ((editingRuleId is null && store.State.Rules.Count + store.State.MarketRules.Count >= 50) || store.State.Rules.Any(rule => rule.Id != editingRuleId && (isTarget
            ? rule.Kind == AlertRuleKind.Target && rule.Symbol == symbol && rule.TargetDirection == targetDirectionValue && rule.TargetPriceText == target
            : rule.Kind == AlertRuleKind.Percentage && rule.Symbol == symbol && rule.WindowMinutes == (int)ruleWindow.Value && rule.ThresholdText == threshold)))
        {
            MessageBox.Show(this, "规则已存在，或已达到 50 条上限。", "无法添加规则", MessageBoxButtons.OK, MessageBoxIcon.Warning);
            return;
        }
        var created = new AlertRule
        {
            Id = editingRuleId ?? Guid.NewGuid(),
            Symbol = symbol,
            Kind = isTarget ? AlertRuleKind.Target : AlertRuleKind.Percentage,
            WindowMinutes = isTarget ? 0 : (int)ruleWindow.Value,
            ThresholdText = isTarget ? "0" : threshold,
            TargetDirection = isTarget ? targetDirectionValue : null,
            TargetPriceText = isTarget ? target : null,
        };
        if (editingRuleId is { } editingId)
        {
            var index = store.State.Rules.FindIndex(rule => rule.Id == editingId);
            if (index < 0) return;
            created.Enabled = store.State.Rules[index].Enabled;
            if (created.Enabled) monitor.InitializeRule(created);
            store.State.Rules[index] = created;
        }
        else
        {
            monitor.InitializeRule(created);
            store.State.Rules.Add(created);
        }
        store.RecordRecentSymbol(symbol);
        store.Save();
        RefreshRules();
        CancelRuleEditing();
        monitor.SubscriptionsChanged();
    }

    private void BeginSelectedRuleEditing()
    {
        if (rulesList.SelectedItems.Count > 0) BeginRuleEditing(rulesList.SelectedItems[0]);
    }

    private void BeginRuleEditing(ListViewItem item)
    {
        if (item.Tag is string marketTag && marketTag.StartsWith("market:", StringComparison.Ordinal) &&
            Guid.TryParse(marketTag[7..], out var marketId))
        {
            var marketRule = store.State.MarketRules.First(value => value.Id == marketId);
            editingMarketRuleId = marketId;
            editingRuleId = null;
            ConfigureRuleKinds("全市场");
            ruleWindow.Value = marketRule.WindowMinutes;
            ruleThreshold.Value = marketRule.Threshold;
            saveRuleButton.Text = "保存修改";
            cancelRuleEditButton.Visible = true;
            return;
        }
        if (item.Tag is not Guid id) return;
        var rule = store.State.Rules.First(value => value.Id == id);
        editingRuleId = id;
        editingMarketRuleId = null;
        ruleSymbol.Text = rule.Symbol;
        ConfigureRuleKinds("单合约", "目标价格");
        ruleKind.SelectedItem = rule.Kind == AlertRuleKind.Target ? "目标价格" : "单合约";
        ruleWindow.Value = Math.Clamp(rule.WindowMinutes == 0 ? 5 : rule.WindowMinutes, 1, 60);
        ruleThreshold.Value = Math.Clamp(rule.Threshold, ruleThreshold.Minimum, ruleThreshold.Maximum);
        targetDirection.SelectedIndex = rule.TargetDirection == TargetDirection.Below ? 1 : 0;
        if (rule.TargetPrice is { } target) targetPrice.Value = Math.Clamp(target, targetPrice.Minimum, targetPrice.Maximum);
        saveRuleButton.Text = "保存修改";
        cancelRuleEditButton.Visible = true;
    }

    private void CancelRuleEditing()
    {
        editingRuleId = null;
        editingMarketRuleId = null;
        ruleSymbol.Text = store.State.PrimarySymbol;
        ConfigureRuleKinds("单合约", "全市场", "目标价格");
        ruleWindow.Value = 5;
        ruleThreshold.Value = 3;
        targetDirection.SelectedIndex = 0;
        saveRuleButton.Text = "添加规则";
        cancelRuleEditButton.Visible = false;
    }

    private void ToggleSelectedRule()
    {
        if (rulesList.SelectedItems.Count == 0) return;
        if (rulesList.SelectedItems[0].Tag is string marketTag && marketTag.StartsWith("market:", StringComparison.Ordinal) &&
            Guid.TryParse(marketTag[7..], out var marketId))
        {
            var marketRule = store.State.MarketRules.First(item => item.Id == marketId);
            marketRule.Enabled = !marketRule.Enabled;
            store.Save();
            RefreshRules();
            monitor.SubscriptionsChanged();
            return;
        }
        if (rulesList.SelectedItems[0].Tag is not Guid id) return;
        var rule = store.State.Rules.First(item => item.Id == id);
        rule.Enabled = !rule.Enabled;
        rule.RiseTriggered = false;
        rule.FallTriggered = false;
        rule.TargetTriggered = false;
        if (rule.Enabled) monitor.InitializeRule(rule);
        store.Save();
        RefreshRules();
        monitor.SubscriptionsChanged();
    }

    private void DeleteSelectedRule()
    {
        if (rulesList.SelectedItems.Count == 0) return;
        if (rulesList.SelectedItems[0].Tag is string marketTag && marketTag.StartsWith("market:", StringComparison.Ordinal) &&
            Guid.TryParse(marketTag[7..], out var marketId))
        {
            store.State.MarketRules.RemoveAll(rule => rule.Id == marketId);
            store.Save();
            RefreshRules();
            monitor.SubscriptionsChanged();
            return;
        }
        if (rulesList.SelectedItems[0].Tag is not Guid id) return;
        store.State.Rules.RemoveAll(rule => rule.Id == id);
        store.Save();
        RefreshRules();
        monitor.SubscriptionsChanged();
    }

    private void RefreshRules()
    {
        rulesList.BeginUpdate();
        rulesList.Items.Clear();
        foreach (var rule in store.State.Rules)
        {
            rulesList.Items.Add(new ListViewItem(new[] {
                rule.Symbol,
                rule.Kind == AlertRuleKind.Target
                    ? $"{(rule.TargetDirection == TargetDirection.Above ? "达到或高于" : "达到或低于")} {rule.TargetPriceText}"
                    : $"{rule.WindowMinutes} 分钟内上涨或下跌 ≥ {rule.ThresholdText}%",
                rule.Enabled ? "已启用" : "已暂停",
            }) { Tag = rule.Id, ForeColor = rule.Enabled ? Ink : Muted, ImageIndex = 0 });
        }
        foreach (var rule in store.State.MarketRules)
        {
            rulesList.Items.Add(new ListViewItem(new[] {
                "全部 USDT 永续", $"{rule.WindowMinutes} 分钟内上涨或下跌 ≥ {rule.ThresholdText}%",
                rule.Enabled ? "已启用" : "已暂停",
            }) { Tag = $"market:{rule.Id}", ForeColor = rule.Enabled ? Ink : Muted, ImageIndex = 0 });
        }
        rulesList.EndUpdate();
    }

    private void UpdateRuleFields()
    {
        var market = ruleKind.Text == "全市场";
        var target = ruleKind.Text == "目标价格";
        ruleSymbol.Enabled = !market;
        if (ruleWindowField is not null) ruleWindowField.Visible = !target;
        if (ruleThresholdField is not null) ruleThresholdField.Visible = !target;
        if (targetDirectionField is not null) targetDirectionField.Visible = target;
        if (targetPriceField is not null) targetPriceField.Visible = target;
    }

    private void ConfigureRuleKinds(params string[] values)
    {
        ruleKind.BeginUpdate();
        ruleKind.Items.Clear();
        ruleKind.Items.AddRange(values);
        ruleKind.SelectedIndex = 0;
        ruleKind.EndUpdate();
    }

    private void UpdateTraySymbolList()
    {
        updatingTraySymbols = true;
        traySymbols.Items.Clear();
        foreach (var contract in ContractOrdering.Ordered(contracts, store.State.RecentSymbols, traySearch.Text))
            traySymbols.Items.Add(contract.Symbol, store.State.TraySymbols.Contains(contract.Symbol, StringComparer.OrdinalIgnoreCase));
        updatingTraySymbols = false;
    }

    private void TraySymbolChecked(object? sender, ItemCheckEventArgs eventArgs)
    {
        if (updatingTraySymbols) return;
        var symbol = traySymbols.Items[eventArgs.Index]?.ToString();
        if (symbol is null) return;
        var selected = store.State.TraySymbols.ToHashSet(StringComparer.OrdinalIgnoreCase);
        if (eventArgs.NewValue == CheckState.Checked)
        {
            if (selected.Count >= 3)
            {
                eventArgs.NewValue = CheckState.Unchecked;
                System.Media.SystemSounds.Beep.Play();
                return;
            }
            selected.Add(symbol);
            store.RecordRecentSymbol(symbol);
        }
        else selected.Remove(symbol);
        store.State.TraySymbols = selected.Order(StringComparer.OrdinalIgnoreCase).ToList();
        store.Save();
        monitor.SubscriptionsChanged();
        BeginInvoke((Action)UpdateTraySymbolList);
    }

    private static TabPage Page(string title) => new(title) { BackColor = Color.White, ForeColor = Ink };

    private static Label TextLabel(string text, float size, FontStyle style) => new()
    {
        Text = text, AutoSize = true, ForeColor = Ink, Font = new Font("Segoe UI", size, style),
    };

    private static Panel Field(string label, Control control)
    {
        var panel = new Panel
        {
            Width = control.Width + 12,
            Height = 56,
            Margin = new Padding(0, 0, 10, 0),
            BackColor = Surface,
        };
        var fieldLabel = TextLabel(label, 8.5f, FontStyle.Regular);
        fieldLabel.ForeColor = Muted;
        fieldLabel.Location = new Point(0, 0);
        control.Location = new Point(0, 22);
        panel.Controls.Add(fieldLabel);
        panel.Controls.Add(control);
        return panel;
    }

    private static void ConfigureList(ListView list, string[] columns, int[] widths)
    {
        list.Dock = DockStyle.Fill;
        list.View = View.Details;
        list.FullRowSelect = true;
        list.GridLines = false;
        list.HideSelection = false;
        list.BorderStyle = BorderStyle.None;
        list.BackColor = Surface;
        list.ForeColor = Ink;
        list.HeaderStyle = ColumnHeaderStyle.Nonclickable;
        list.Font = new Font("Segoe UI Variable Text", 9.5f);
        for (var index = 0; index < columns.Length; index++) list.Columns.Add(columns[index], widths[index]);
    }

    private void RefreshContractCombos()
    {
        updatingContractCombos = true;
        var primary = primarySymbol.Text.Length == 0 ? store.State.PrimarySymbol : primarySymbol.Text;
        var rule = ruleSymbol.Text.Length == 0 ? store.State.PrimarySymbol : ruleSymbol.Text;
        var symbols = ContractOrdering.Ordered(contracts, store.State.RecentSymbols).Select(item => item.Symbol).ToArray();
        ConfigureContractCombo(primarySymbol, symbols, primary);
        ConfigureContractCombo(ruleSymbol, symbols, rule);
        updatingContractCombos = false;
    }

    private static void ConfigureContractCombo(ComboBox combo, string[] symbols, string selected)
    {
        combo.BeginUpdate();
        combo.Items.Clear();
        combo.Items.AddRange(symbols);
        combo.AutoCompleteMode = AutoCompleteMode.SuggestAppend;
        combo.AutoCompleteSource = AutoCompleteSource.ListItems;
        combo.Text = selected;
        combo.EndUpdate();
    }

    private static Bitmap CreateEditIcon()
    {
        var bitmap = new Bitmap(16, 16);
        using var graphics = Graphics.FromImage(bitmap);
        graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.AntiAlias;
        using var pen = new Pen(Muted, 2.2f) { StartCap = System.Drawing.Drawing2D.LineCap.Round, EndCap = System.Drawing.Drawing2D.LineCap.Round };
        graphics.DrawLine(pen, 4, 12, 11.5f, 4.5f);
        graphics.DrawLine(pen, 3, 13, 5.5f, 12.5f);
        graphics.DrawLine(pen, 10.5f, 4, 12.5f, 6);
        return bitmap;
    }

    private static void StylePrimaryButton(Button button)
    {
        button.FlatStyle = FlatStyle.Flat;
        button.FlatAppearance.BorderSize = 0;
        button.BackColor = Primary;
        button.ForeColor = Color.White;
        button.Padding = new Padding(12, 7, 12, 7);
        button.Cursor = Cursors.Hand;
        button.FlatAppearance.MouseOverBackColor = PrimaryHover;
        button.FlatAppearance.MouseDownBackColor = PrimaryHover;
    }

    private static void StyleSecondaryButton(Button button)
    {
        button.FlatStyle = FlatStyle.Flat;
        button.FlatAppearance.BorderColor = Divider;
        button.FlatAppearance.BorderSize = 1;
        button.BackColor = Color.White;
        button.ForeColor = Ink;
        button.Padding = new Padding(10, 6, 10, 6);
        button.Cursor = Cursors.Hand;
        button.FlatAppearance.MouseOverBackColor = PrimarySoft;
    }

    private static Control ListSection(string title, ListView list, Padding? margin = null)
    {
        var layout = new TableLayoutPanel
        {
            Dock = DockStyle.Fill, ColumnCount = 1, RowCount = 2,
            Margin = margin ?? Padding.Empty,
        };
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        var heading = TextLabel(title, 10, FontStyle.Bold);
        heading.Margin = new Padding(2, 0, 0, 8);
        layout.Controls.Add(heading, 0, 0);
        layout.Controls.Add(list, 0, 1);
        return layout;
    }

    private static void DrawTab(TabControl tabs, DrawItemEventArgs eventArgs)
    {
        var selected = eventArgs.Index == tabs.SelectedIndex;
        var bounds = eventArgs.Bounds;
        using var background = new SolidBrush(selected ? Color.White : Surface);
        using var foreground = new SolidBrush(selected ? Ink : Muted);
        eventArgs.Graphics.FillRectangle(background, bounds);
        var text = tabs.TabPages[eventArgs.Index].Text;
        using var tabFont = new Font("Segoe UI Variable Text", 10, selected ? FontStyle.Bold : FontStyle.Regular);
        TextRenderer.DrawText(
            eventArgs.Graphics, text, tabFont,
            bounds, selected ? Ink : Muted,
            TextFormatFlags.HorizontalCenter | TextFormatFlags.VerticalCenter | TextFormatFlags.SingleLine
        );
        if (selected)
        {
            using var accent = new SolidBrush(Primary);
            eventArgs.Graphics.FillRectangle(accent, bounds.Left + 18, bounds.Bottom - 3, bounds.Width - 36, 3);
        }
    }
}
