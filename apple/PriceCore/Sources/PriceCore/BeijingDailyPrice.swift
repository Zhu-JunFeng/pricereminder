import Foundation

public enum BeijingDailyPrice {
    public static func startMilliseconds(at date: Date) -> Int64 {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 8 * 3_600)!
        return Int64(calendar.startOfDay(for: date).timeIntervalSince1970 * 1_000)
    }

    package static func decodeOpen(from data: Data, start: Int64) throws -> Decimal {
        guard let rows = try JSONSerialization.jsonObject(with: data) as? [[Any]],
              let row = rows.first, row.count >= 2,
              let timestamp = row[0] as? NSNumber, timestamp.int64Value == start,
              let text = row[1] as? String,
              let price = Decimal(string: text, locale: Locale(identifier: "en_US_POSIX")),
              price > 0 else { throw PriceCoreError.invalidExchangeResponse }
        return price
    }
}
