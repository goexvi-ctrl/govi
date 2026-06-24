import Cocoa

// ColorParser turns a color specification into NSColor. Supported forms:
//   #RGB, #RRGGBB
//   named colors from ColorNames (X11-style, case-insensitive)
// An empty string means "use the system default" (nil).
enum ColorParser {
    static func parse(_ spec: String) -> NSColor? {
        let s = spec.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.isEmpty { return nil }
        if s.hasPrefix("#") {
            return parseHex(s)
        }
        let key = s.lowercased().filter { !$0.isWhitespace }
        guard let rgb = ColorNames.rgb[key] else { return nil }
        return calibrated(rgb.0, rgb.1, rgb.2)
    }

    private static func parseHex(_ spec: String) -> NSColor? {
        var hex = String(spec.dropFirst())
        if hex.count == 3 {
            hex = hex.map { String($0) + String($0) }.joined()
        }
        guard hex.count == 6, hex.allSatisfy({ $0.isHexDigit }) else { return nil }
        var value: UInt64 = 0
        guard Scanner(string: hex).scanHexInt64(&value) else { return nil }
        let r = UInt8((value >> 16) & 0xFF)
        let g = UInt8((value >> 8) & 0xFF)
        let b = UInt8(value & 0xFF)
        return calibrated(r, g, b)
    }

    private static func calibrated(_ r: UInt8, _ g: UInt8, _ b: UInt8) -> NSColor {
        NSColor(calibratedRed: CGFloat(r) / 255,
                green: CGFloat(g) / 255,
                blue: CGFloat(b) / 255,
                alpha: 1)
    }
}