import Cocoa

// ColorParser turns a color specification into NSColor. Supported forms:
//   #RGB, #RRGGBB
//   named colors from ColorCatalog (compact or readable; case/spacing/apostrophe/hyphen insensitive)
// An empty string means "use the system default" (nil).
enum ColorParser {
    private static let nameLookup: [String: (UInt8, UInt8, UInt8)] = buildNameLookup()
    private static let compactByNormalized: [String: String] = buildCompactByNormalized()

    static func normalizeName(_ s: String) -> String {
        ColorCatalog.normalizeName(s)
    }

    static func parse(_ spec: String) -> NSColor? {
        let s = spec.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.isEmpty { return nil }
        if s.hasPrefix("#") {
            return parseHex(s)
        }
        guard let rgb = nameLookup[normalizeName(s)] else { return nil }
        return calibrated(rgb.0, rgb.1, rgb.2)
    }

    // readableName returns the display form for a named color spec, or nil for hex/unknown.
    static func readableName(for spec: String) -> String? {
        let s = spec.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.isEmpty || s.hasPrefix("#") { return nil }
        guard let compact = compactByNormalized[normalizeName(s)] else { return nil }
        return ColorCatalog.readableByCompact[compact]
    }

    // storageSpec is the value to persist: readable names for known colors, hex as-is, empty for default.
    static func storageSpec(_ raw: String) -> String {
        let s = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.isEmpty { return "" }
        if s.hasPrefix("#") { return s }
        if let readable = readableName(for: s) { return readable }
        return s
    }

    // displaySpec is what Settings shows: readable names for known colors, hex/empty unchanged.
    static func displaySpec(_ spec: String) -> String {
        readableName(for: spec) ?? spec.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func buildNameLookup() -> [String: (UInt8, UInt8, UInt8)] {
        var lookup: [String: (UInt8, UInt8, UInt8)] = [:]
        for color in ColorCatalog.colors {
            let rgb = (color.r, color.g, color.b)
            lookup[normalizeName(color.compact)] = rgb
            lookup[normalizeName(color.readable)] = rgb
        }
        return lookup
    }

    private static func buildCompactByNormalized() -> [String: String] {
        var map: [String: String] = [:]
        for color in ColorCatalog.colors {
            map[normalizeName(color.compact)] = color.compact
            map[normalizeName(color.readable)] = color.compact
        }
        return map
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