import SwiftUI

enum AppTheme {
    static let primary = Color(red: 0.0, green: 0.50, blue: 0.54)
    static let rise = Color(red: 0.09, green: 0.54, blue: 0.33)
    static let fall = Color(red: 0.78, green: 0.24, blue: 0.24)
    static let surface = Color.primary.opacity(0.045)
}

extension View {
    func numericPriceStyle() -> some View {
        fontDesign(.rounded).monospacedDigit()
    }
}
