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
    private static let openFilesInKey = "openFilesIn"

    enum OpenFilesIn: String {
        case newWindow
        case tab
    }

    // openFilesIn chooses whether files opened from the launcher, Finder, or
    // File > Open appear in a new window or as a tab in the frontmost window.
    static var openFilesIn: OpenFilesIn {
        get {
            let d = UserDefaults.standard
            guard let raw = d.string(forKey: openFilesInKey),
                  let mode = OpenFilesIn(rawValue: raw) else {
                return .newWindow
            }
            return mode
        }
        set {
            UserDefaults.standard.set(newValue.rawValue, forKey: openFilesInKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    private static let warnCloseKey = "warnOnUnsavedClose"

    // warnOnUnsavedClose prompts before closing a window or tab with unsaved edits.
    static var warnOnUnsavedClose: Bool {
        get {
            let d = UserDefaults.standard
            guard d.object(forKey: warnCloseKey) != nil else { return true }
            return d.bool(forKey: warnCloseKey)
        }
        set {
            UserDefaults.standard.set(newValue, forKey: warnCloseKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

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
    private let openFilesPopup = NSPopUpButton()
    private let warnCloseCheckbox = NSButton(checkboxWithTitle: "Warn before closing unsaved files", target: nil, action: nil)
    private static let maxPadding: Double = 64

    private init() {
        let win = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 420, height: 190),
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

        let openLabel = NSTextField(labelWithString: "Open files in:")
        openLabel.translatesAutoresizingMaskIntoConstraints = false

        openFilesPopup.translatesAutoresizingMaskIntoConstraints = false
        openFilesPopup.addItems(withTitles: ["New window", "Tab of front window"])
        openFilesPopup.target = self
        openFilesPopup.action = #selector(openFilesChanged)

        warnCloseCheckbox.translatesAutoresizingMaskIntoConstraints = false
        warnCloseCheckbox.target = self
        warnCloseCheckbox.action = #selector(warnCloseChanged)

        content.addSubview(label)
        content.addSubview(field)
        content.addSubview(stepper)
        content.addSubview(openLabel)
        content.addSubview(openFilesPopup)
        content.addSubview(warnCloseCheckbox)

        NSLayoutConstraint.activate([
            label.leadingAnchor.constraint(equalTo: content.leadingAnchor, constant: 20),
            label.topAnchor.constraint(equalTo: content.topAnchor, constant: 24),
            field.leadingAnchor.constraint(equalTo: label.trailingAnchor, constant: 8),
            field.centerYAnchor.constraint(equalTo: label.centerYAnchor),
            field.widthAnchor.constraint(equalToConstant: 56),
            stepper.leadingAnchor.constraint(equalTo: field.trailingAnchor, constant: 4),
            stepper.centerYAnchor.constraint(equalTo: label.centerYAnchor),

            openLabel.leadingAnchor.constraint(equalTo: content.leadingAnchor, constant: 20),
            openLabel.topAnchor.constraint(equalTo: label.bottomAnchor, constant: 20),
            openFilesPopup.leadingAnchor.constraint(equalTo: openLabel.trailingAnchor, constant: 8),
            openFilesPopup.centerYAnchor.constraint(equalTo: openLabel.centerYAnchor),
            openFilesPopup.trailingAnchor.constraint(lessThanOrEqualTo: content.trailingAnchor, constant: -20),

            warnCloseCheckbox.leadingAnchor.constraint(equalTo: content.leadingAnchor, constant: 20),
            warnCloseCheckbox.topAnchor.constraint(equalTo: openLabel.bottomAnchor, constant: 16),
        ])

        syncFromSettings()
    }

    private func syncFromSettings() {
        let v = Double(Settings.padding)
        field.doubleValue = v
        stepper.doubleValue = v
        openFilesPopup.selectItem(at: Settings.openFilesIn == .tab ? 1 : 0)
        warnCloseCheckbox.state = Settings.warnOnUnsavedClose ? .on : .off
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

    @objc private func openFilesChanged() {
        Settings.openFilesIn = openFilesPopup.indexOfSelectedItem == 1 ? .tab : .newWindow
    }

    @objc private func warnCloseChanged() {
        Settings.warnOnUnsavedClose = warnCloseCheckbox.state == .on
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
