using System.Globalization;
using System.Text.Json;
namespace PriceReminder.Windows;
internal sealed class TradeRelayBatch {
 public string Type {get;set;} = "";
 public int Protocol {get;set;}
 public long Version {get;set;}
 public string Session {get;set;} = "";
 public string? EventId {get;set;}
 public long LatestEventTime {get;set;}
 public TradeRelayPrice[] Prices {get;set;} = [];
 public TradeRelayTrigger[] Triggers {get;set;} = [];
 public static TradeRelayBatch Parse(string json) => JsonSerializer.Deserialize<TradeRelayBatch>(json,new JsonSerializerOptions {PropertyNameCaseInsensitive=true}) ?? throw new InvalidDataException("行情协议无效");
}
internal sealed record TradeRelayPrice(string Symbol,string Price,long EventTime);
internal sealed record TradeRelayTrigger(string RuleId,string Symbol,string Direction,string? ChangePct,string? ThresholdPct,int WindowMinutes,string Price,string BaselinePrice,long EventTime) {
 public AlertTrigger Convert() => new(Guid.Parse(RuleId),Symbol,AlertRuleKind.MarketPercentage,Direction=="rise"?TriggerDirection.Rise:TriggerDirection.Fall,ChangePct is null?null:decimal.Parse(ChangePct,CultureInfo.InvariantCulture),ThresholdPct??"",WindowMinutes,null,Price,BaselinePrice,EventTime);
}
