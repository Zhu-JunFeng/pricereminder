---
name: "币价提醒"
description: "冷静、可靠、直接的跨端合约价格提醒工具"
colors:
  primary: "oklch(0.52 0.11 200)"
  primary-soft: "oklch(0.94 0.025 200)"
  background: "oklch(1 0 0)"
  surface: "oklch(0.97 0.006 200)"
  ink: "oklch(0.19 0.018 215)"
  muted: "oklch(0.46 0.018 210)"
  divider: "oklch(0.89 0.008 205)"
  rise: "oklch(0.56 0.15 150)"
  fall: "oklch(0.56 0.18 25)"
  dark-background: "oklch(0.12 0.006 210)"
  dark-surface: "oklch(0.18 0.01 210)"
  dark-ink: "oklch(0.94 0.006 200)"
typography:
  headline:
    fontFamily: "SF Pro Display, Roboto, sans-serif"
    fontSize: "28px"
    fontWeight: 650
    lineHeight: 1.18
    letterSpacing: "-0.02em"
  title:
    fontFamily: "SF Pro Text, Roboto, sans-serif"
    fontSize: "17px"
    fontWeight: 600
    lineHeight: 1.3
  body:
    fontFamily: "SF Pro Text, Roboto, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.45
  label:
    fontFamily: "SF Pro Text, Roboto, sans-serif"
    fontSize: "13px"
    fontWeight: 500
    lineHeight: 1.3
rounded:
  sm: "8px"
  md: "12px"
  pill: "999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "24px"
  xl: "32px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.background}"
    rounded: "{rounded.sm}"
    padding: "12px 16px"
  field:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    padding: "12px 14px"
---

# Design System: 币价提醒

## Overview

**Creative North Star: "安静的行情仪表"**

界面服务于持续监控而不是交易冲动。纯净中性底色承载数据，海蓝绿只出现在主操作、选中状态和连接状态；国际惯例的绿涨红跌只表达结果，不扩散成装饰。布局紧凑但留出稳定节奏，让价格、更新时间、规则状态和异常提示始终比容器更突出。

产品明确拒绝交易所行情大厅、霓虹加密终端和大卡片极简样板。动效只用于连接、保存和状态切换，不做页面入场编排。

**Key Characteristics:**

- 一眼识别实时价格、连接状态与最新更新时间。
- 数字使用等宽数字特性，价格按币安精度展示。
- 浅色与暗色跟随系统，信息结构保持一致。
- 手机单主合约，Mac 菜单栏最多三个合约。

## Colors

海蓝绿是唯一品牌强调；纯白和近黑承担环境适配，涨跌色严格限制在行情状态。

### Primary

- **潮汐蓝绿：** 用于主操作、当前选择、正常连接和焦点环。
- **浅潮面：** 用于选中行与信息提示背景，不用于大面积装饰。

### Neutral

- **清醒白：** 浅色模式主背景。
- **雾面层：** 工具栏、输入区和分组容器。
- **墨蓝黑：** 正文、标题与核心数字。
- **海雾灰：** 次要说明和时间戳，保持可读性。

### Named Rules

**The One Signal Rule.** 海蓝绿在任一屏幕中只表达可操作或已连接状态；装饰性使用被禁止。

**The Market Color Rule.** 绿色只表示上涨，红色只表示下跌，并且必须同时出现箭头或文字。

## Typography

**Display Font:** 平台原生 SF Pro Display / Roboto
**Body Font:** 平台原生 SF Pro Text / Roboto
**Label/Mono Font:** 平台原生等宽数字特性

**Character:** 单一原生字体体系减少跨平台陌生感；通过字重、字号和等宽数字建立精密感，不用展示字体制造品牌噪音。

### Hierarchy

- **Headline**（650，28px，1.18）：页面标题与主价格，不超过两行。
- **Title**（600，17px，1.3）：规则名称、合约符号和分组标题。
- **Body**（400，15px，1.45）：说明、错误和状态解释。
- **Label**（500，13px，1.3）：时间戳、字段名、阈值摘要。

### Named Rules

**The Numeric Rhythm Rule.** 所有变化中的价格使用等宽数字，禁止因位数变化造成界面跳动。

## Elevation

系统默认扁平，通过背景明度和分隔线建立层级。阴影仅用于系统弹层或平台原生菜单，不在静态规则项和行情容器上叠加宽软阴影。

### Named Rules

**The Flat Instrument Rule.** 静态表面没有装饰性投影；如果一个容器需要阴影才能被理解，先修正层级与间距。

## Components

### Buttons

- **Shape:** 紧凑圆角（8px），主按钮可使用全宽但不使用胶囊形。
- **Primary:** 潮汐蓝绿填充、白色文字，垂直内边距 12px。
- **Hover / Focus:** 桌面悬停仅轻微改变明度；焦点使用清晰 2px 品牌色轮廓。
- **Secondary:** 中性表面或文字按钮，不增加另一套强调色。

### Chips

- **Style:** 胶囊仅用于短状态，如“实时”“积累中”“已暂停”。
- **State:** 选中状态使用浅潮面与墨蓝黑文字；错误状态使用低饱和红色背景和明确文字。

### Cards / Containers

- **Corner Style:** 分组容器最大 12px，禁止嵌套卡片。
- **Background:** 页面背景与雾面层形成一级对比。
- **Shadow Strategy:** 静态无阴影。
- **Border:** 需要边界时使用 1px 中性分隔线。
- **Internal Padding:** 16px；高密度规则行使用 12px 垂直节奏。

### Inputs / Fields

- **Style:** 雾面层背景、8px 圆角、清晰标签；数值字段显示单位。
- **Focus:** 品牌色焦点环，不移动布局。
- **Error / Disabled:** 错误解释放在字段附近；禁用状态同时降低对比并禁止操作。

### Navigation

手机使用平台原生底部导航或层级导航，最多三个一级入口：行情、预警、设置。macOS 以菜单栏弹层为主，不复制手机导航。

### Live Price Row

合约符号、按 tickSize 格式化的价格、方向箭头和更新时间排成稳定单行；价格陈旧超过 30 秒时停止使用涨跌色并明确标记“价格已陈旧”。

## Do's and Don'ts

### Do:

- **Do** 优先展示连接、权限、积累和暂停等真实状态。
- **Do** 使用系统亮暗主题与平台原生交互。
- **Do** 让通知内容完整解释合约、窗口、阈值和实际变化。
- **Do** 将系统展示刷新限制为最多每 15 秒一次，同时保持规则引擎每秒判断。

### Don't:

- **Don't** 做交易所行情大厅；禁止榜单、K 线、广告、活动入口和密集红绿数字。
- **Don't** 使用霓虹加密终端、玻璃拟态、发光边框或赛博装饰。
- **Don't** 把每个字段放入独立大圆角卡片或嵌套卡片。
- **Don't** 使用渐变文字、彩色侧边条或无意义装饰动效。
- **Don't** 隐藏断线、陈旧价格、通知权限关闭或数据不足。
