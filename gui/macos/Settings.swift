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

    private static let selModeKey = "selectionMode"

    // SelMode controls whether a mouse selection captures typed/pasted input.
    // Mirrors the engine's selmode option (and its 0/1/2 bridge codes).
    enum SelMode: String, CaseIterable {
        case traditional
        case wysiwyg
        case combined

        var label: String {
            switch self {
            case .traditional: return "Traditional (selection is copy-only)"
            case .wysiwyg: return "WYSIWYG (typing replaces the selection)"
            case .combined: return "Combined (replace only while inserting)"
            }
        }

        // code is the 0/1/2 value the bridge (GoviSetSelMode/GoviSelMode) uses.
        var code: Int32 {
            switch self {
            case .traditional: return 0
            case .wysiwyg: return 1
            case .combined: return 2
            }
        }
    }

    // selMode is the GUI default for the engine's selmode option, applied to new
    // windows before LoadStartup (so .exrc can override) and live to open windows.
    static var selMode: SelMode {
        get {
            let d = UserDefaults.standard
            guard let raw = d.string(forKey: selModeKey),
                  let mode = SelMode(rawValue: raw) else {
                return .combined
            }
            return mode
        }
        set {
            UserDefaults.standard.set(newValue.rawValue, forKey: selModeKey)
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

    private static let useTabsKey = "useTabs"

    // useTabs controls macOS window tabbing. When off, every editor is a
    // standalone window and no tab bar is shown (even with "prefer tabs" on).
    static var useTabs: Bool {
        get {
            let d = UserDefaults.standard
            guard d.object(forKey: useTabsKey) != nil else { return true }
            return d.bool(forKey: useTabsKey)
        }
        set {
            UserDefaults.standard.set(newValue, forKey: useTabsKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    private static let defaultRowsKey = "defaultTextRows"
    private static let defaultColsKey = "defaultColumns"
    private static let showDimensionsKey = "showDimensionsInTitle"
    private static let fontFamilyKey = "editorFontFamily"
    private static let fontSizeKey = "editorFontSize"
    private static let fgColorKey = "foregroundColor"
    private static let bgColorKey = "backgroundColor"

    static let initialTextRows = 24
    static let initialColumns = 80
    static let defaultFontSize: CGFloat = 14
    static let minTextRows = 8
    static let maxTextRows = 200
    static let minColumns = 20
    static let maxColumns = 512
    static let minFontSize: CGFloat = 8
    static let maxFontSize: CGFloat = 72

    enum EditorFontFamily: String, CaseIterable {
        case system
        case menlo = "Menlo"
        case monaco = "Monaco"
        case courier = "Courier"
        case courierNew = "Courier New"
        case sfMono = "SFMono-Regular"

        var label: String {
            switch self {
            case .system: return "System Monospaced"
            case .menlo: return "Menlo"
            case .monaco: return "Monaco"
            case .courier: return "Courier"
            case .courierNew: return "Courier New"
            case .sfMono: return "SF Mono"
            }
        }

        func font(size: CGFloat) -> NSFont {
            switch self {
            case .system:
                return NSFont.monospacedSystemFont(ofSize: size, weight: .regular)
            default:
                return NSFont(name: rawValue, size: size)
                    ?? NSFont.monospacedSystemFont(ofSize: size, weight: .regular)
            }
        }
    }

    // defaultTextRows and defaultColumns are the editable grid size for new
    // windows (rows x cols, not counting the status line).
    static var defaultTextRows: Int {
        get {
            let d = UserDefaults.standard
            guard d.object(forKey: defaultRowsKey) != nil else { return initialTextRows }
            return clampInt(d.integer(forKey: defaultRowsKey), min: minTextRows, max: maxTextRows)
        }
        set {
            UserDefaults.standard.set(clampInt(newValue, min: minTextRows, max: maxTextRows),
                                      forKey: defaultRowsKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    static var defaultColumns: Int {
        get {
            let d = UserDefaults.standard
            guard d.object(forKey: defaultColsKey) != nil else { return initialColumns }
            return clampInt(d.integer(forKey: defaultColsKey), min: minColumns, max: maxColumns)
        }
        set {
            UserDefaults.standard.set(clampInt(newValue, min: minColumns, max: maxColumns),
                                      forKey: defaultColsKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    // showDimensionsInTitle shows the live rows×cols in the window title bar
    // (not in tab labels); all tabs in a window share the same dimensions.
    static var showDimensionsInTitle: Bool {
        get {
            let d = UserDefaults.standard
            guard d.object(forKey: showDimensionsKey) != nil else { return false }
            return d.bool(forKey: showDimensionsKey)
        }
        set {
            UserDefaults.standard.set(newValue, forKey: showDimensionsKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    static var fontFamily: EditorFontFamily {
        get {
            let d = UserDefaults.standard
            guard let raw = d.string(forKey: fontFamilyKey),
                  let family = EditorFontFamily(rawValue: raw) else {
                return .system
            }
            return family
        }
        set {
            UserDefaults.standard.set(newValue.rawValue, forKey: fontFamilyKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    static var fontSize: CGFloat {
        get {
            let d = UserDefaults.standard
            guard d.object(forKey: fontSizeKey) != nil else { return defaultFontSize }
            return clampWindow(d.double(forKey: fontSizeKey), min: minFontSize, max: maxFontSize)
        }
        set {
            UserDefaults.standard.set(Double(clampWindow(newValue, min: minFontSize, max: maxFontSize)),
                                      forKey: fontSizeKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    static var editorFont: NSFont {
        fontFamily.font(size: fontSize)
    }

    // defaultForegroundColorSpec and defaultBackgroundColorSpec are applied to
    // new tabs at creation; :set and .exrc may override per tab.
    static var defaultForegroundColorSpec: String {
        get { UserDefaults.standard.string(forKey: fgColorKey) ?? "" }
        set { UserDefaults.standard.set(newValue, forKey: fgColorKey) }
    }

    static var defaultBackgroundColorSpec: String {
        get { UserDefaults.standard.string(forKey: bgColorKey) ?? "" }
        set { UserDefaults.standard.set(newValue, forKey: bgColorKey) }
    }

    static var defaultForegroundColor: NSColor {
        ColorParser.parse(defaultForegroundColorSpec) ?? NSColor.textColor
    }

    static var defaultBackgroundColor: NSColor {
        ColorParser.parse(defaultBackgroundColorSpec) ?? NSColor.textBackgroundColor
    }

    private static let initialDirKey = "initialDirectory"

    // initialDirectory is the working directory for new fileless windows (Finder
    // launch, File > New). Stored raw; empty means "use the home directory".
    static var initialDirectory: String {
        get { UserDefaults.standard.string(forKey: initialDirKey) ?? "" }
        set { UserDefaults.standard.set(newValue, forKey: initialDirKey) }
    }

    // resolvedInitialDirectory is the absolute directory to apply (home when unset).
    static var resolvedInitialDirectory: String {
        initialDirectory.isEmpty
            ? NSHomeDirectory()
            : (initialDirectory as NSString).expandingTildeInPath
    }

    private static let cursorStyleKey = "cursorStyle"
    private static let cursorColorKey = "cursorColor"

    enum CursorStyle: String, CaseIterable {
        case box
        case bar

        var label: String {
            switch self {
            case .box: return "Box"
            case .bar: return "Vertical bar"
            }
        }
    }

    // cursorStyle selects how the editor cursor is drawn: a filled box (the
    // classic vi block) or a thin vertical bar at the insertion point.
    static var cursorStyle: CursorStyle {
        get {
            let d = UserDefaults.standard
            guard let raw = d.string(forKey: cursorStyleKey),
                  let s = CursorStyle(rawValue: raw) else { return .box }
            return s
        }
        set {
            UserDefaults.standard.set(newValue.rawValue, forKey: cursorStyleKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    // cursorColorSpec is the cursor color (empty = system accent blue).
    static var cursorColorSpec: String {
        get { UserDefaults.standard.string(forKey: cursorColorKey) ?? "" }
        set {
            UserDefaults.standard.set(newValue, forKey: cursorColorKey)
            NotificationCenter.default.post(name: changed, object: nil)
        }
    }

    static var cursorColor: NSColor {
        ColorParser.parse(cursorColorSpec) ?? .systemBlue
    }

    private static func clampWindow(_ value: CGFloat, min: CGFloat, max: CGFloat) -> CGFloat {
        Swift.min(Swift.max(value, min), max)
    }

    private static func clampWindow(_ value: Double, min: CGFloat, max: CGFloat) -> CGFloat {
        clampWindow(CGFloat(value), min: min, max: max)
    }

    private static func clampInt(_ value: Int, min: Int, max: Int) -> Int {
        Swift.min(Swift.max(value, min), max)
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

    private let paddingField = NSTextField()
    private let paddingStepper = NSStepper()
    private let rowsField = NSTextField()
    private let rowsStepper = NSStepper()
    private let colsField = NSTextField()
    private let colsStepper = NSStepper()
    private let fontPopup = NSPopUpButton()
    private let fontSizeField = NSTextField()
    private let fontSizeStepper = NSStepper()
    private let openFilesPopup = NSPopUpButton()
    private let selModePopup = NSPopUpButton()
    private let useTabsCheckbox = NSButton(checkboxWithTitle: "Use window tabs (show the tab bar)", target: nil, action: nil)
    private let warnCloseCheckbox = NSButton(checkboxWithTitle: "Warn before closing unsaved files", target: nil, action: nil)
    private let showDimensionsCheckbox = NSButton(checkboxWithTitle: "Show rows×columns in title bar (not tabs)", target: nil, action: nil)
    private let fgColorField = NSTextField()
    private let bgColorField = NSTextField()
    private let fgColorSwatch = NSView()
    private let bgColorSwatch = NSView()
    private let initialDirField = NSTextField()
    private let cursorStylePopup = NSPopUpButton()
    private let cursorColorField = NSTextField()
    private let cursorColorSwatch = NSView()
    private static let maxPadding: Double = 64

    private init() {
        let win = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 520, height: 560),
            styleMask: [.titled, .closable], backing: .buffered, defer: false)
        win.title = "Settings"
        super.init(window: win)
        buildUI()
    }

    required init?(coder: NSCoder) { fatalError("not supported") }

    private func buildUI() {
        guard let content = window?.contentView else { return }

        let paddingRow = makeNumericRow(
            label: "Text padding (pixels):", field: paddingField, stepper: paddingStepper,
            min: 0, max: Self.maxPadding, action: #selector(paddingChanged))
        let rowsRow = makeNumericRow(
            label: "Default rows:", field: rowsField, stepper: rowsStepper,
            min: Double(Settings.minTextRows), max: Double(Settings.maxTextRows),
            action: #selector(defaultRowsChanged))
        let colsRow = makeNumericRow(
            label: "Default columns:", field: colsField, stepper: colsStepper,
            min: Double(Settings.minColumns), max: Double(Settings.maxColumns),
            action: #selector(defaultColsChanged))
        let fontSizeRow = makeNumericRow(
            label: "Font size (points):", field: fontSizeField, stepper: fontSizeStepper,
            min: Double(Settings.minFontSize), max: Double(Settings.maxFontSize),
            action: #selector(fontSizeChanged))

        let fontLabel = NSTextField(labelWithString: "Font:")
        fontLabel.translatesAutoresizingMaskIntoConstraints = false
        fontPopup.translatesAutoresizingMaskIntoConstraints = false
        fontPopup.addItems(withTitles: Settings.EditorFontFamily.allCases.map(\.label))
        fontPopup.target = self
        fontPopup.action = #selector(fontFamilyChanged)
        let fontRow = NSStackView(views: [fontLabel, fontPopup])
        fontRow.translatesAutoresizingMaskIntoConstraints = false
        fontRow.alignment = .centerY
        fontRow.spacing = 8

        let openLabel = NSTextField(labelWithString: "Open files in:")
        openLabel.translatesAutoresizingMaskIntoConstraints = false
        openFilesPopup.translatesAutoresizingMaskIntoConstraints = false
        openFilesPopup.addItems(withTitles: ["New window", "Tab of front window"])
        openFilesPopup.target = self
        openFilesPopup.action = #selector(openFilesChanged)
        let openRow = NSStackView(views: [openLabel, openFilesPopup])
        openRow.translatesAutoresizingMaskIntoConstraints = false
        openRow.alignment = .centerY
        openRow.spacing = 8

        let selModeLabel = NSTextField(labelWithString: "Selection editing:")
        selModeLabel.translatesAutoresizingMaskIntoConstraints = false
        selModePopup.translatesAutoresizingMaskIntoConstraints = false
        selModePopup.addItems(withTitles: Settings.SelMode.allCases.map(\.label))
        selModePopup.target = self
        selModePopup.action = #selector(selModeChanged)
        let selModeRow = NSStackView(views: [selModeLabel, selModePopup])
        selModeRow.translatesAutoresizingMaskIntoConstraints = false
        selModeRow.alignment = .centerY
        selModeRow.spacing = 8

        useTabsCheckbox.translatesAutoresizingMaskIntoConstraints = false
        useTabsCheckbox.target = self
        useTabsCheckbox.action = #selector(useTabsChanged)
        warnCloseCheckbox.translatesAutoresizingMaskIntoConstraints = false
        warnCloseCheckbox.target = self
        warnCloseCheckbox.action = #selector(warnCloseChanged)
        showDimensionsCheckbox.translatesAutoresizingMaskIntoConstraints = false
        showDimensionsCheckbox.target = self
        showDimensionsCheckbox.action = #selector(showDimensionsChanged)

        let fgRow = makeColorRow(label: "Default foreground (new tabs):", field: fgColorField, swatch: fgColorSwatch,
                                 placeholder: "#RRGGBB or color name (empty = system)")
        let bgRow = makeColorRow(label: "Default background (new tabs):", field: bgColorField, swatch: bgColorSwatch,
                                 placeholder: "#RRGGBB or color name (empty = system)")

        let dirLabel = NSTextField(labelWithString: "Initial directory (new windows):")
        dirLabel.translatesAutoresizingMaskIntoConstraints = false
        initialDirField.translatesAutoresizingMaskIntoConstraints = false
        initialDirField.placeholderString = "empty = home (~)"
        initialDirField.widthAnchor.constraint(equalToConstant: 220).isActive = true
        initialDirField.delegate = self
        let chooseDir = NSButton(title: "Choose…", target: self, action: #selector(chooseInitialDir))
        chooseDir.translatesAutoresizingMaskIntoConstraints = false
        let dirRow = NSStackView(views: [dirLabel, initialDirField, chooseDir])
        dirRow.translatesAutoresizingMaskIntoConstraints = false
        dirRow.alignment = .centerY
        dirRow.spacing = 8

        let cursorLabel = NSTextField(labelWithString: "Cursor:")
        cursorLabel.translatesAutoresizingMaskIntoConstraints = false
        cursorStylePopup.translatesAutoresizingMaskIntoConstraints = false
        cursorStylePopup.addItems(withTitles: Settings.CursorStyle.allCases.map(\.label))
        cursorStylePopup.target = self
        cursorStylePopup.action = #selector(cursorStyleChanged)
        let cursorStyleRow = NSStackView(views: [cursorLabel, cursorStylePopup])
        cursorStyleRow.translatesAutoresizingMaskIntoConstraints = false
        cursorStyleRow.alignment = .centerY
        cursorStyleRow.spacing = 8

        let cursorColorRow = makeColorRow(label: "Cursor color:", field: cursorColorField,
                                          swatch: cursorColorSwatch,
                                          placeholder: "#RRGGBB or color name (empty = blue)")

        let stack = NSStackView(views: [
            paddingRow, rowsRow, colsRow, fontRow, fontSizeRow, fgRow, bgRow, dirRow, openRow,
            selModeRow, cursorStyleRow, cursorColorRow,
            useTabsCheckbox, showDimensionsCheckbox, warnCloseCheckbox,
        ])
        stack.translatesAutoresizingMaskIntoConstraints = false
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 16
        content.addSubview(stack)

        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: content.leadingAnchor, constant: 20),
            stack.trailingAnchor.constraint(lessThanOrEqualTo: content.trailingAnchor, constant: -20),
            stack.topAnchor.constraint(equalTo: content.topAnchor, constant: 20),
            stack.bottomAnchor.constraint(lessThanOrEqualTo: content.bottomAnchor, constant: -20),
        ])

        [paddingField, rowsField, colsField, fontSizeField, fgColorField, bgColorField, cursorColorField]
            .forEach { $0.delegate = self }
        syncFromSettings()
    }

    private func makeColorRow(
        label text: String, field: NSTextField, swatch: NSView, placeholder: String
    ) -> NSStackView {
        let label = NSTextField(labelWithString: text)
        field.translatesAutoresizingMaskIntoConstraints = false
        field.placeholderString = placeholder
        field.widthAnchor.constraint(equalToConstant: 220).isActive = true
        swatch.translatesAutoresizingMaskIntoConstraints = false
        swatch.wantsLayer = true
        swatch.layer?.borderWidth = 1
        swatch.layer?.borderColor = NSColor.separatorColor.cgColor
        NSLayoutConstraint.activate([
            swatch.widthAnchor.constraint(equalToConstant: 28),
            swatch.heightAnchor.constraint(equalToConstant: 20),
        ])
        let row = NSStackView(views: [label, field, swatch])
        row.translatesAutoresizingMaskIntoConstraints = false
        row.alignment = .centerY
        row.spacing = 8
        return row
    }

    private func makeNumericRow(
        label text: String, field: NSTextField, stepper: NSStepper,
        min: Double, max: Double, action: Selector
    ) -> NSStackView {
        let label = NSTextField(labelWithString: text)
        field.translatesAutoresizingMaskIntoConstraints = false
        field.alignment = .right
        field.widthAnchor.constraint(equalToConstant: 64).isActive = true
        stepper.translatesAutoresizingMaskIntoConstraints = false
        stepper.minValue = min
        stepper.maxValue = max
        stepper.increment = 1
        stepper.valueWraps = false
        stepper.target = self
        stepper.action = action
        let row = NSStackView(views: [label, field, stepper])
        row.translatesAutoresizingMaskIntoConstraints = false
        row.alignment = .centerY
        row.spacing = 8
        return row
    }

    private func syncFromSettings() {
        syncNumeric(paddingField, paddingStepper, value: Double(Settings.padding))
        syncNumeric(rowsField, rowsStepper, value: Double(Settings.defaultTextRows))
        syncNumeric(colsField, colsStepper, value: Double(Settings.defaultColumns))
        syncNumeric(fontSizeField, fontSizeStepper, value: Double(Settings.fontSize))
        if let idx = Settings.EditorFontFamily.allCases.firstIndex(of: Settings.fontFamily) {
            fontPopup.selectItem(at: idx)
        }
        openFilesPopup.selectItem(at: Settings.openFilesIn == .tab ? 1 : 0)
        if let idx = Settings.SelMode.allCases.firstIndex(of: Settings.selMode) {
            selModePopup.selectItem(at: idx)
        }
        useTabsCheckbox.state = Settings.useTabs ? .on : .off
        warnCloseCheckbox.state = Settings.warnOnUnsavedClose ? .on : .off
        showDimensionsCheckbox.state = Settings.showDimensionsInTitle ? .on : .off
        fgColorField.stringValue = Settings.defaultForegroundColorSpec
        bgColorField.stringValue = Settings.defaultBackgroundColorSpec
        updateSwatch(fgColorSwatch, spec: Settings.defaultForegroundColorSpec,
                     fallback: Settings.defaultForegroundColor)
        updateSwatch(bgColorSwatch, spec: Settings.defaultBackgroundColorSpec,
                     fallback: Settings.defaultBackgroundColor)
        initialDirField.stringValue = Settings.initialDirectory
        if let idx = Settings.CursorStyle.allCases.firstIndex(of: Settings.cursorStyle) {
            cursorStylePopup.selectItem(at: idx)
        }
        cursorColorField.stringValue = Settings.cursorColorSpec
        updateSwatch(cursorColorSwatch, spec: Settings.cursorColorSpec, fallback: Settings.cursorColor)
    }

    private func updateSwatch(_ swatch: NSView, spec: String, fallback: NSColor) {
        let c = ColorParser.parse(spec) ?? fallback
        swatch.layer?.backgroundColor = c.cgColor
    }

    private func syncNumeric(_ field: NSTextField, _ stepper: NSStepper, value: Double) {
        field.doubleValue = value
        stepper.doubleValue = value
    }

    private func commitNumeric(
        _ field: NSTextField, _ stepper: NSStepper, lo: Double, hi: Double, raw: Double,
        assign: (CGFloat) -> Void
    ) {
        let v = Swift.min(Swift.max(raw, lo), hi)
        field.doubleValue = v
        stepper.doubleValue = v
        assign(CGFloat(v))
    }

    @objc private func paddingChanged() {
        commitNumeric(paddingField, paddingStepper, lo: 0, hi: Self.maxPadding,
                      raw: paddingStepper.doubleValue, assign: { Settings.padding = $0 })
    }

    @objc private func defaultRowsChanged() {
        commitInt(rowsField, rowsStepper,
                  lo: Settings.minTextRows, hi: Settings.maxTextRows,
                  raw: Int(rowsStepper.intValue), assign: { Settings.defaultTextRows = $0 })
    }

    @objc private func defaultColsChanged() {
        commitInt(colsField, colsStepper,
                  lo: Settings.minColumns, hi: Settings.maxColumns,
                  raw: Int(colsStepper.intValue), assign: { Settings.defaultColumns = $0 })
    }

    @objc private func fontSizeChanged() {
        commitNumeric(fontSizeField, fontSizeStepper,
                      lo: Double(Settings.minFontSize), hi: Double(Settings.maxFontSize),
                      raw: fontSizeStepper.doubleValue, assign: { Settings.fontSize = $0 })
    }

    @objc private func fontFamilyChanged() {
        let idx = fontPopup.indexOfSelectedItem
        guard idx >= 0, idx < Settings.EditorFontFamily.allCases.count else { return }
        Settings.fontFamily = Settings.EditorFontFamily.allCases[idx]
    }

    @objc private func chooseInitialDir() {
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        guard panel.runModal() == .OK, let url = panel.url else { return }
        Settings.initialDirectory = url.path
        initialDirField.stringValue = url.path
    }

    // commitInitialDir validates the typed directory (empty = home) and stores it.
    private func commitInitialDir() {
        let raw = initialDirField.stringValue.trimmingCharacters(in: .whitespacesAndNewlines)
        if !raw.isEmpty {
            var isDir: ObjCBool = false
            let expanded = (raw as NSString).expandingTildeInPath
            if !FileManager.default.fileExists(atPath: expanded, isDirectory: &isDir) || !isDir.boolValue {
                NSSound.beep()
                initialDirField.stringValue = Settings.initialDirectory
                return
            }
        }
        Settings.initialDirectory = raw
    }

    @objc private func openFilesChanged() {
        Settings.openFilesIn = openFilesPopup.indexOfSelectedItem == 1 ? .tab : .newWindow
    }

    @objc private func selModeChanged() {
        let idx = selModePopup.indexOfSelectedItem
        guard idx >= 0, idx < Settings.SelMode.allCases.count else { return }
        Settings.selMode = Settings.SelMode.allCases[idx]
    }

    @objc private func cursorStyleChanged() {
        let idx = cursorStylePopup.indexOfSelectedItem
        guard idx >= 0, idx < Settings.CursorStyle.allCases.count else { return }
        Settings.cursorStyle = Settings.CursorStyle.allCases[idx]
    }

    @objc private func useTabsChanged() {
        Settings.useTabs = useTabsCheckbox.state == .on
    }

    @objc private func warnCloseChanged() {
        Settings.warnOnUnsavedClose = warnCloseCheckbox.state == .on
    }

    @objc private func showDimensionsChanged() {
        Settings.showDimensionsInTitle = showDimensionsCheckbox.state == .on
    }

    private func commitInt(
        _ field: NSTextField, _ stepper: NSStepper, lo: Int, hi: Int, raw: Int,
        assign: (Int) -> Void
    ) {
        let v = Swift.min(Swift.max(raw, lo), hi)
        field.intValue = Int32(v)
        stepper.intValue = Int32(v)
        assign(v)
    }

    func controlTextDidEndEditing(_ obj: Notification) {
        guard let field = obj.object as? NSTextField else { return }
        switch field {
        case paddingField:
            commitNumeric(paddingField, paddingStepper, lo: 0, hi: Self.maxPadding,
                          raw: field.doubleValue, assign: { Settings.padding = $0 })
        case rowsField:
            commitInt(rowsField, rowsStepper,
                      lo: Settings.minTextRows, hi: Settings.maxTextRows,
                      raw: Int(field.intValue), assign: { Settings.defaultTextRows = $0 })
        case colsField:
            commitInt(colsField, colsStepper,
                      lo: Settings.minColumns, hi: Settings.maxColumns,
                      raw: Int(field.intValue), assign: { Settings.defaultColumns = $0 })
        case fontSizeField:
            commitNumeric(fontSizeField, fontSizeStepper,
                          lo: Double(Settings.minFontSize), hi: Double(Settings.maxFontSize),
                          raw: field.doubleValue, assign: { Settings.fontSize = $0 })
        case initialDirField:
            commitInitialDir()
        case fgColorField:
            commitColor(fgColorField, fgColorSwatch, fallback: NSColor.textColor,
                        assign: { Settings.defaultForegroundColorSpec = $0 })
        case bgColorField:
            commitColor(bgColorField, bgColorSwatch, fallback: NSColor.textBackgroundColor,
                        assign: { Settings.defaultBackgroundColorSpec = $0 })
        case cursorColorField:
            commitColor(cursorColorField, cursorColorSwatch, fallback: NSColor.systemBlue,
                        assign: { Settings.cursorColorSpec = $0 })
        default:
            break
        }
    }

    private func commitColor(
        _ field: NSTextField, _ swatch: NSView, fallback: NSColor, assign: (String) -> Void
    ) {
        let raw = field.stringValue.trimmingCharacters(in: .whitespacesAndNewlines)
        if !raw.isEmpty && ColorParser.parse(raw) == nil {
            NSSound.beep()
            syncFromSettings()
            return
        }
        assign(raw)
        updateSwatch(swatch, spec: raw, fallback: fallback)
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
