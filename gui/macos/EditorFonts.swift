import AppKit

// EditorFonts lists every fixed-pitch font installed on the system and resolves
// the PostScript name stored in Settings to an NSFont.
enum EditorFonts {
    struct Choice {
        let postScriptName: String
        let displayName: String
    }

    static let systemDisplayName = "System Monospaced"

    private static let legacyPostScriptNames: [String: String] = [
        "system": "",
        "Menlo": "Menlo-Regular",
        "Monaco": "Monaco",
        "Courier": "Courier",
        "Courier New": "CourierNewPSMT",
        "SFMono-Regular": "SFMono-Regular",
    ]

    static let choices: [Choice] = {
        var result = [Choice(postScriptName: "", displayName: systemDisplayName)]
        result.append(contentsOf: fixedPitchChoices())
        return result
    }()

    static func migrateStoredName(_ raw: String) -> String {
        if raw.isEmpty { return "" }
        if let legacy = legacyPostScriptNames[raw] { return legacy }
        if choices.contains(where: { $0.postScriptName == raw }) { return raw }
        if let match = choices.first(where: { $0.displayName == raw }) { return match.postScriptName }
        if let font = NSFont(name: raw, size: 12),
           NSFontManager.shared.traits(of: font).contains(.fixedPitchFontMask) {
            return raw
        }
        if let match = choices.first(where: {
            $0.displayName.caseInsensitiveCompare(raw) == .orderedSame
        }) {
            return match.postScriptName
        }
        return ""
    }

    static func font(postScriptName: String, size: CGFloat) -> NSFont {
        if postScriptName.isEmpty {
            return NSFont.monospacedSystemFont(ofSize: size, weight: .regular)
        }
        return NSFont(name: postScriptName, size: size)
            ?? NSFont.monospacedSystemFont(ofSize: size, weight: .regular)
    }

    static func displayName(forPostScriptName name: String) -> String {
        if name.isEmpty { return systemDisplayName }
        return choices.first(where: { $0.postScriptName == name })?.displayName
            ?? (NSFont(name: name, size: 12)?.familyName ?? name)
    }

    private static func fixedPitchChoices() -> [Choice] {
        let manager = NSFontManager.shared
        guard let names = manager.availableFontNames(with: .fixedPitchFontMask) else { return [] }

        var byFamily: [String: (postScriptName: String, face: String)] = [:]
        for name in names {
            guard let font = NSFont(name: name, size: 12) else { continue }
            let family = font.familyName ?? name
            let face = font.displayName ?? name
            if let existing = byFamily[family] {
                if rank(face: face) < rank(face: existing.face) {
                    byFamily[family] = (name, face)
                }
            } else {
                byFamily[family] = (name, face)
            }
        }

        return byFamily.keys.sorted {
            $0.localizedCaseInsensitiveCompare($1) == .orderedAscending
        }.map { family in
            let entry = byFamily[family]!
            return Choice(postScriptName: entry.postScriptName, displayName: family)
        }
    }

    // rank prefers Regular/Book/Roman faces when picking one font per family.
    private static func rank(face: String) -> Int {
        let lower = face.lowercased()
        if lower.contains("regular") { return 0 }
        if lower.contains("book") { return 1 }
        if lower.contains("roman") { return 2 }
        if lower.contains("medium") { return 3 }
        return 4
    }
}