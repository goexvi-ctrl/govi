import Cocoa

// Settings holds the app's user-configurable preferences, persisted in
// UserDefaults. Changing a value posts Settings.changed so open editor views
// can re-read and relayout.
enum Settings {
    static let changed = Notification.Name("GoviSettingsChanged")

    private static let paddingKey = "padding"
    static let defaultPadding: CGFloat = 3

    // padding is the inset, in pixels, between the window edge and the text.
    static var padding: CGFloat {
        get {
            let d = UserDefaults.standard
            guard d.object(forKey: paddingKey) != nil else { return defaultPadding }
            return CGFloat(d.double(forKey: paddingKey))
        }
        set {
            UserDefaults.standard.set(Double(max(0, newValue)), forKey: paddingKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    private static let spellKey = "checkSpellingWhileTyping"

    // spellChecking controls continuous spell checking (red squiggles).
    static var spellChecking: Bool {
        get {
            let d = UserDefaults.standard
            guard d.object(forKey: spellKey) != nil else { return true }
            return d.bool(forKey: spellKey)
        }
        set {
            UserDefaults.standard.set(newValue, forKey: spellKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }
}

// SettingsWindowController is the single Settings window (Cmd-,). It edits the
// padding via a text field and stepper; changes apply live to all open windows.
final class SettingsWindowController: NSWindowController, NSTextFieldDelegate {
    static let shared = SettingsWindowController()

    private let field = NSTextField()
    private let stepper = NSStepper()
    private static let maxPadding: Double = 64

    private init() {
        let win = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 340, height: 110),
            styleMask: [.titled, .closable], backing: .buffered, defer: false)
        win.title = "Settings"
        super.init(window: win)
        buildUI()
    }

    required init?(coder: NSCoder) { fatalError("not supported") }

    private func buildUI() {
        guard let content = window?.contentView else { return }

        let label = NSTextField(labelWithString: "Text padding (pixels):")
        label.translatesAutoresizingMaskIntoConstraints = false

        field.translatesAutoresizingMaskIntoConstraints = false
        field.alignment = .right
        field.delegate = self

        stepper.translatesAutoresizingMaskIntoConstraints = false
        stepper.minValue = 0
        stepper.maxValue = Self.maxPadding
        stepper.increment = 1
        stepper.valueWraps = false
        stepper.target = self
        stepper.action = #selector(stepperChanged)

        content.addSubview(label)
        content.addSubview(field)
        content.addSubview(stepper)

        NSLayoutConstraint.activate([
            label.leadingAnchor.constraint(equalTo: content.leadingAnchor, constant: 20),
            label.centerYAnchor.constraint(equalTo: content.centerYAnchor),
            field.leadingAnchor.constraint(equalTo: label.trailingAnchor, constant: 8),
            field.centerYAnchor.constraint(equalTo: content.centerYAnchor),
            field.widthAnchor.constraint(equalToConstant: 56),
            stepper.leadingAnchor.constraint(equalTo: field.trailingAnchor, constant: 4),
            stepper.centerYAnchor.constraint(equalTo: content.centerYAnchor),
        ])

        syncFromSettings()
    }

    private func syncFromSettings() {
        let v = Double(Settings.padding)
        field.doubleValue = v
        stepper.doubleValue = v
    }

    private func commit(_ raw: Double) {
        let v = min(max(0, raw), Self.maxPadding)
        field.doubleValue = v
        stepper.doubleValue = v
        Settings.padding = CGFloat(v)
    }

    @objc private func stepperChanged() {
        commit(stepper.doubleValue)
    }

    func controlTextDidEndEditing(_ obj: Notification) {
        commit(field.doubleValue)
    }

    func show() {
        syncFromSettings()
        if !(window?.isVisible ?? false) {
            window?.center()
        }
        showWindow(nil)
        window?.makeKeyAndOrderFront(nil)
    }
}
