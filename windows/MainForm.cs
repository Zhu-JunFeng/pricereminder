using System.Globalization;

namespace PriceReminder.Windows;

internal sealed class MainForm : Form
{
    private static readonly Color Primary = Color.FromArgb(0, 127, 136);
    private static readonly Color Ink = Color.FromArgb(23, 42, 46);
    private static readonly Color Muted = Color.FromArgb(82, 103, 107);
    private static readonly Color Surface = Color.FromArgb(244, 248, 248);
    private static readonly Color Divider = Color.FromArgb(213, 222, 223);
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
    private readonly ListView rulesList = new();
    private readonly ListView historyList = new();
    private readonly CheckedListBox traySymbols = new();
    private readonly Label contractsError = new();
    private readonly CheckBox startupCheck = new();
    private IReadOnlyList<Contract> contracts = [];
    private bool updatingTraySymbols;

    public MainForm(LocalStore store, MonitorService monitor)
    {
        this.store = store;
        this.monitor = monitor;
        Text = "币价提醒";
        Icon = BrandIcon.Create();
        StartPosition = FormStartPosition.CenterScreen;
        MinimumSize = new Size(720, 560);
        Size = new Size(780, 640);
        BackColor = Color.White;
        Font = new Font("Segoe UI", 10);

        var tabs = new TabControl { Dock = DockStyle.Fill, Padding = new Point(14, 7) };
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
        var symbols = values.Select(item => item.Symbol).ToArray();
        ConfigureContractCombo(primarySymbol, symbols, store.State.PrimarySymbol);
        ConfigureContractCombo(ruleSymbol, symbols, store.State.PrimarySymbol);
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
        sourceLabel.ForeColor = snapshot.Connected ? Primary : Color.FromArgb(173, 104, 24);
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
    }

    public void RefreshHistory()
    {
        historyList.BeginUpdate();
        historyList.Items.Clear();
        foreach (var item in store.State.History.Take(100))
        {
            var direction = item.Direction == TriggerDirection.Rise ? "↑ 上涨" : "↓ 下跌";
            historyList.Items.Add(new ListViewItem(new[] {
                item.Symbol, direction, $"{item.ChangePercent:F2}%", item.PriceText,
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
        layout.Controls.Add(primarySymbol, 0, 1);
        sourceLabel.AutoSize = true;
        sourceLabel.Anchor = AnchorStyles.Right;
        sourceLabel.Font = new Font("Segoe UI", 9, FontStyle.Bold);
        layout.Controls.Add(sourceLabel, 1, 1);
        symbolLabel.AutoSize = true;
        symbolLabel.ForeColor = Muted;
        layout.Controls.Add(symbolLabel, 0, 2);
        priceLabel.AutoSize = true;
        priceLabel.Font = new Font("Cascadia Mono", 30, FontStyle.Bold);
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
        layout.Controls.Add(statusLabel, 0, 6);
        monitorButton.Text = "停止监控";
        monitorButton.AutoSize = true;
        monitorButton.Anchor = AnchorStyles.Right;
        StylePrimaryButton(monitorButton);
        monitorButton.Click += (_, _) => { if (monitor.Running) monitor.Stop(); else monitor.Start(); };
        layout.Controls.Add(monitorButton, 1, 6);
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

        var addRow = new FlowLayoutPanel { Dock = DockStyle.Fill, AutoSize = true, Padding = new Padding(0, 14, 0, 12), WrapContents = false };
        ruleSymbol.Width = 180;
        ruleSymbol.DropDownStyle = ComboBoxStyle.DropDown;
        ruleWindow.Minimum = 1; ruleWindow.Maximum = 60; ruleWindow.Value = 5; ruleWindow.Width = 72;
        ruleThreshold.Minimum = .1m; ruleThreshold.Maximum = 100; ruleThreshold.DecimalPlaces = 1; ruleThreshold.Increment = .1m; ruleThreshold.Value = 3; ruleThreshold.Width = 82;
        var addButton = new Button { Text = "添加规则", AutoSize = true };
        StylePrimaryButton(addButton);
        addButton.Click += (_, _) => AddRule();
        addRow.Controls.AddRange(new Control[] {
            Field("合约", ruleSymbol), Field("分钟", ruleWindow), Field("变化 %", ruleThreshold), addButton,
        });
        layout.Controls.Add(addRow, 0, 1);

        ConfigureList(rulesList, ["合约", "规则", "状态"], [130, 350, 90]);
        rulesList.DoubleClick += (_, _) => ToggleSelectedRule();
        rulesList.KeyDown += (_, eventArgs) =>
        {
            if (eventArgs.KeyCode == Keys.Delete) DeleteSelectedRule();
            if (eventArgs.KeyCode == Keys.Space) ToggleSelectedRule();
        };
        var rulesPanel = new Panel { Dock = DockStyle.Fill };
        rulesPanel.Controls.Add(rulesList);
        var rulesMenu = new ContextMenuStrip();
        rulesMenu.Items.Add("启用 / 暂停", null, (_, _) => ToggleSelectedRule());
        rulesMenu.Items.Add("删除", null, (_, _) => DeleteSelectedRule());
        rulesList.ContextMenuStrip = rulesMenu;
        layout.Controls.Add(rulesPanel, 0, 2);

        var historyPanel = new Panel { Dock = DockStyle.Fill, Padding = new Padding(0, 18, 0, 0) };
        ConfigureList(historyList, ["合约", "方向", "变化", "价格", "时间"], [115, 85, 90, 150, 120]);
        historyPanel.Controls.Add(historyList);
        layout.Controls.Add(historyPanel, 0, 3);
        page.Controls.Add(layout);
        return page;
    }

    private TabPage CreateSettingsPage()
    {
        var page = Page("设置");
        var layout = new TableLayoutPanel
        {
            Dock = DockStyle.Fill, Padding = new Padding(28, 24, 28, 24), ColumnCount = 2, RowCount = 4,
        };
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 54));
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 46));
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
        traySymbols.Dock = DockStyle.Fill;
        traySymbols.CheckOnClick = true;
        traySymbols.BorderStyle = BorderStyle.FixedSingle;
        traySymbols.ItemCheck += TraySymbolChecked;
        layout.Controls.Add(traySymbols, 0, 2);

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
        details.Controls.Add(detailFlow);
        layout.Controls.Add(details, 1, 2);
        var source = TextLabel("行情来源：币安 U 本位永续最新成交价；终端不通时使用服务端转发的同一币安行情。", 9, FontStyle.Regular);
        source.ForeColor = Muted;
        source.Margin = new Padding(0, 18, 0, 0);
        source.MaximumSize = new Size(650, 0);
        layout.Controls.Add(source, 0, 3);
        layout.SetColumnSpan(source, 2);
        page.Controls.Add(layout);
        return page;
    }

    private void SelectPrimary()
    {
        var symbol = primarySymbol.Text.Trim().ToUpperInvariant();
        if (!contracts.Any(item => item.Symbol == symbol)) return;
        store.State.PrimarySymbol = symbol;
        store.Save();
        monitor.SubscriptionsChanged();
    }

    private void AddRule()
    {
        var symbol = ruleSymbol.Text.Trim().ToUpperInvariant();
        if (!contracts.Any(item => item.Symbol == symbol))
        {
            MessageBox.Show(this, "请选择有效的 U 本位永续合约。", "无法添加规则", MessageBoxButtons.OK, MessageBoxIcon.Warning);
            return;
        }
        var threshold = ruleThreshold.Value.ToString("0.0", CultureInfo.InvariantCulture);
        if (store.State.Rules.Count >= 50 || store.State.Rules.Any(rule =>
            rule.Symbol == symbol && rule.WindowMinutes == (int)ruleWindow.Value && rule.ThresholdText == threshold))
        {
            MessageBox.Show(this, "规则已存在，或已达到 50 条上限。", "无法添加规则", MessageBoxButtons.OK, MessageBoxIcon.Warning);
            return;
        }
        store.State.Rules.Add(new AlertRule
        {
            Symbol = symbol,
            WindowMinutes = (int)ruleWindow.Value,
            ThresholdText = threshold,
        });
        store.Save();
        RefreshRules();
        monitor.SubscriptionsChanged();
    }

    private void ToggleSelectedRule()
    {
        if (rulesList.SelectedItems.Count == 0 || rulesList.SelectedItems[0].Tag is not Guid id) return;
        var rule = store.State.Rules.First(item => item.Id == id);
        rule.Enabled = !rule.Enabled;
        rule.RiseTriggered = false;
        rule.FallTriggered = false;
        store.Save();
        RefreshRules();
        monitor.SubscriptionsChanged();
    }

    private void DeleteSelectedRule()
    {
        if (rulesList.SelectedItems.Count == 0 || rulesList.SelectedItems[0].Tag is not Guid id) return;
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
                $"{rule.WindowMinutes} 分钟内上涨或下跌 ≥ {rule.ThresholdText}%",
                rule.Enabled ? "已启用" : "已暂停",
            }) { Tag = rule.Id, ForeColor = rule.Enabled ? Ink : Muted });
        }
        rulesList.EndUpdate();
    }

    private void UpdateTraySymbolList()
    {
        updatingTraySymbols = true;
        traySymbols.Items.Clear();
        foreach (var contract in contracts)
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
        }
        else selected.Remove(symbol);
        store.State.TraySymbols = selected.Order(StringComparer.OrdinalIgnoreCase).ToList();
        store.Save();
        monitor.SubscriptionsChanged();
    }

    private static TabPage Page(string title) => new(title) { BackColor = Color.White, ForeColor = Ink };

    private static Label TextLabel(string text, float size, FontStyle style) => new()
    {
        Text = text, AutoSize = true, ForeColor = Ink, Font = new Font("Segoe UI", size, style),
    };

    private static Panel Field(string label, Control control)
    {
        var panel = new Panel { Width = control.Width + 12, Height = 56, Margin = new Padding(0, 0, 10, 0) };
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
        list.BorderStyle = BorderStyle.FixedSingle;
        for (var index = 0; index < columns.Length; index++) list.Columns.Add(columns[index], widths[index]);
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

    private static void StylePrimaryButton(Button button)
    {
        button.FlatStyle = FlatStyle.Flat;
        button.FlatAppearance.BorderSize = 0;
        button.BackColor = Primary;
        button.ForeColor = Color.White;
        button.Padding = new Padding(12, 7, 12, 7);
        button.Cursor = Cursors.Hand;
    }
}
